package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"

	"math"
	"sort"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

type Document struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ChatRequest struct {
	ChatID  string `json:"chat_id"`
	Message string `json:"message"`
}

type ChatMessage struct {
	ID        int64  `json:"id"`
	ChatID    string `json:"chat_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content GeminiContent `json:"content"`
	} `json:"candidates"`
}

type EmbeddingRequest struct {
	Content  GeminiContent `json:"content"`
	TaskType string        `json:"taskType"`
}

type EmbeddingResponse struct {
	Embedding struct {
		Values []float64 `json:"values"`
	} `json:"embedding"`
}

type RetrievedChunk struct {
	ID         int     `json:"id"`
	DocumentID string  `json:"document_id"`
	ChunkText  string  `json:"chunk_text"`
	Similarity float64 `json:"similarity"`
}

var db *sql.DB

func initDB() {
	var err error

	db, err = sql.Open("sqlite3", "./knowledge.db")

	if err != nil {
		log.Fatal(err)
	}
	err = db.Ping()

	if err != nil {
		log.Fatal(err)
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS documents (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	status TEXT NOT NULL
	);
	`

	_, err = db.Exec(createTableQuery)

	if err != nil {
		log.Fatal("Error creating documents table:", err)
	}

	_, err = db.Exec(`
    ALTER TABLE documents
    ADD COLUMN extracted_text TEXT
`)

	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Fatal("Error adding extracted_text column:", err)
	}

	log.Println("Documents table created successfully")

	log.Println("Database connected successfully")

	createChatTables := `
	CREATE TABLE IF NOT EXISTS chats (
	id TEXT PRIMARY KEY,
	created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY (chat_id) REFERENCES chats(id)
	);
	`

	_, err = db.Exec(createChatTables)

	if err != nil {
		log.Fatal("Error creating chat tables:", err)
	}

	log.Println("Chat table created successfully")

	createChunksTable := `
CREATE TABLE IF NOT EXISTS document_chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id TEXT NOT NULL,
    chunk_text TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    FOREIGN KEY (document_id) REFERENCES documents(id)
);
`

	_, err = db.Exec(createChunksTable)

	if err != nil {
		log.Fatal("Error creating document chunks table:", err)
	}

	log.Println("Document chunks table created successfully")

	_, err = db.Exec(`
    ALTER TABLE document_chunks
    ADD COLUMN embedding TEXT
`)

	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Fatal("Error adding embedding column:", err)
	}

	log.Println("Embedding column added successfully")
}

func listDocuments(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	rows, err := db.Query(
		"SELECT id, name, status FROM documents",
	)

	if err != nil {
		http.Error(w, "Could not retrieve documents", http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	documents := make([]Document, 0)

	for rows.Next() {
		var doc Document

		err := rows.Scan(
			&doc.ID,
			&doc.Name,
			&doc.Status,
		)

		if err != nil {
			http.Error(w, "Could not read document details", http.StatusInternalServerError)
			return
		}

		documents = append(documents, doc)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error reading documents", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(documents)

}

func deleteDocument(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	id := r.PathValue("id")

	var docID string

	err := db.QueryRow(
		"SELECT id FROM documents WHERE id = ?",
		id,
	).Scan(&docID)

	if err == sql.ErrNoRows {
		http.Error(w, "Document not found", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	files, err := filepath.Glob(
		filepath.Join("uploads", id+".*"),
	)
	log.Println("Document ID:", id)
	log.Println("Files found:", files)

	if err != nil {
		http.Error(w, "Could not locate document file", http.StatusInternalServerError)
		return
	}

	for _, filePath := range files {
		err := os.Remove(filePath)

		if err != nil {
			http.Error(w, "Could not delete document file", http.StatusInternalServerError)
			return
		}
	}

	_, err = db.Exec(
		"DELETE FROM documents WHERE id = ?",
		id,
	)

	if err != nil {
		http.Error(w, "Could not delete document record", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]string{
		"message": "Document deleted successfully",
		"id":      id,
	})

}

func uploadDocument(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Please upload a file using file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	extension := strings.ToLower(filepath.Ext(header.Filename))

	if extension != ".pdf" && extension != ".txt" {
		http.Error(w, "Only PDF and TXT files are allowed", http.StatusBadRequest)
		return
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	err = os.MkdirAll("uploads", 0755)
	if err != nil {
		http.Error(w, "Could not create uploads folder", http.StatusInternalServerError)
		return
	}

	savedName := id + extension
	filePath := filepath.Join("uploads", savedName)

	destination, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Could not save uploaded file", http.StatusInternalServerError)
		return

	}

	_, err = io.Copy(destination, file)
	closeErr := destination.Close()

	if err != nil || closeErr != nil {
		os.Remove(filePath)
		http.Error(w, "Could not write uploaded file", http.StatusInternalServerError)
		return

	}

	var extractedText string

	if extension == ".txt" {

		extractedText, err = extractTextFromTXT(filePath)

	} else if extension == ".pdf" {

		extractedText, err = extractTextFromPDF(filePath)

	}

	if err != nil {
		os.Remove(filePath)
		http.Error(w, "Could not extract text from document", http.StatusInternalServerError)
		return
	}

	log.Println("Extracted document text:", extractedText)

	doc := Document{
		ID:     id,
		Name:   filepath.Base(header.Filename),
		Status: "Uploaded",
	}

	_, err = db.Exec(
		`INSERT INTO documents (id, name, status, extracted_text)
     VALUES (?, ?, ?, ?)`,
		doc.ID,
		doc.Name,
		doc.Status,
		extractedText,
	)

	if err != nil {
		os.Remove(filePath)
		http.Error(w, "Could not save document details", http.StatusInternalServerError)
		return

	}

	chunks := splitIntoChunks(extractedText)

	log.Println("Total chunks created:", len(chunks))

	for i, chunk := range chunks {

		_, err = db.Exec(
			`INSERT INTO document_chunks
        (document_id, chunk_text, chunk_index)
        VALUES (?, ?, ?)`,
			id,
			chunk,
			i+1,
		)

		if err != nil {
			log.Println("Error saving chunk:", err)
			http.Error(w, "Could not save document chunks", http.StatusInternalServerError)
			return
		}

		log.Printf("Chunk %d saved successfully", i+1)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(doc)
}

func extractTextFromTXT(filePath string) (string, error) {

	content, err := os.ReadFile(filePath)

	if err != nil {
		return "", err
	}

	return string(content), nil
}

func extractTextFromPDF(filePath string) (string, error) {

	f, reader, err := pdf.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	textReader, err := reader.GetPlainText()
	if err != nil {
		return "", err
	}

	var buffer bytes.Buffer

	_, err = buffer.ReadFrom(textReader)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

func splitIntoChunks(textContent string) []string {

	words := strings.Fields(textContent)

	const chunkSize = 500
	const overlap = 50

	var chunks []string

	for start := 0; start < len(words); {

		end := start + chunkSize

		if end > len(words) {
			end = len(words)
		}

		chunk := strings.Join(words[start:end], " ")

		chunks = append(chunks, chunk)

		if end == len(words) {
			break
		}

		start = end - overlap
	}

	return chunks
}

func getDocumentChunks(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	id := strings.TrimPrefix(r.URL.Path, "/api/documents/")
	id = strings.TrimSuffix(id, "/chunks")

	rows, err := db.Query(
		`SELECT id, document_id, chunk_text, chunk_index
         FROM document_chunks
         WHERE document_id = ?
         ORDER BY chunk_index`,
		id,
	)

	if err != nil {
		http.Error(w, "Could not retrieve document chunks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	chunks := make([]map[string]interface{}, 0)

	for rows.Next() {

		var chunkID int
		var documentID string
		var chunkText string
		var chunkIndex int

		err := rows.Scan(&chunkID, &documentID, &chunkText, &chunkIndex)

		if err != nil {
			http.Error(w, "Could not read document chunks", http.StatusInternalServerError)
			return
		}

		chunks = append(chunks, map[string]interface{}{
			"id":          chunkID,
			"document_id": documentID,
			"chunk_text":  chunkText,
			"chunk_index": chunkIndex,
		})
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error reading document chunks", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(chunks)
}

func generateGeminiResponse(userMessage string) (string, error) {

	apiKey := os.Getenv("GEMINI_API_KEY")

	requestData := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: userMessage},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return "", err
	}

	apiURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash-lite:generateContent"

	client := &http.Client{
		Timeout: 90 * time.Second,
	}

	var lastErr error

	for attempt := 1; attempt <= 3; attempt++ {

		ctx, cancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			apiURL,
			bytes.NewReader(jsonData),
		)

		if err != nil {
			cancel()
			return "", err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", apiKey)

		response, err := client.Do(req)

		if err != nil {
			cancel()
			lastErr = err
		} else {

			responseBody, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			cancel()

			if readErr != nil {
				return "", readErr
			}

			if response.StatusCode == http.StatusOK {

				var geminiResponse GeminiResponse

				err = json.Unmarshal(responseBody, &geminiResponse)
				if err != nil {
					return "", err
				}

				if len(geminiResponse.Candidates) == 0 ||
					len(geminiResponse.Candidates[0].Content.Parts) == 0 {
					return "", fmt.Errorf("Gemini returned no response")
				}

				return geminiResponse.Candidates[0].Content.Parts[0].Text, nil
			}

			lastErr = fmt.Errorf(
				"Gemini API HTTP %d: %s",
				response.StatusCode,
				string(responseBody),
			)

			// Retry only temporary server errors.
			if response.StatusCode != http.StatusServiceUnavailable {
				return "", lastErr
			}
		}

		if attempt < 3 {
			delay := time.Duration(1<<attempt) * time.Second

			log.Printf(
				"Gemini attempt %d failed: %v. Retrying in %v",
				attempt,
				lastErr,
				delay,
			)

			time.Sleep(delay)
		}
	}

	return "", fmt.Errorf(
		"Gemini failed after 3 attempts: %w",
		lastErr,
	)
}

func generateEmbedding(textContent string, taskType string) ([]float64, error) {

	apiKey := os.Getenv("GEMINI_API_KEY")

	apiURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent"

	embeddingRequest := EmbeddingRequest{
		Content: GeminiContent{
			Parts: []GeminiPart{
				{
					Text: textContent,
				},
			},
		},
		TaskType: taskType,
	}

	jsonData, err := json.Marshal(embeddingRequest)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		apiURL,
		bytes.NewBuffer(jsonData),
	)

	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("embedding API error: %s", string(body))
	}

	var embeddingResponse EmbeddingResponse

	err = json.NewDecoder(response.Body).Decode(&embeddingResponse)
	if err != nil {
		return nil, err
	}

	return embeddingResponse.Embedding.Values, nil
}

func processPendingEmbeddings() error {

	rows, err := db.Query(`
        SELECT id, chunk_text
        FROM document_chunks
        WHERE embedding IS NULL OR embedding = ''
    `)

	if err != nil {
		return err
	}

	type PendingChunk struct {
		ID   int
		Text string
	}

	var pendingChunks []PendingChunk

	for rows.Next() {
		var chunk PendingChunk

		err := rows.Scan(&chunk.ID, &chunk.Text)
		if err != nil {
			rows.Close()
			return err
		}

		pendingChunks = append(pendingChunks, chunk)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}

	rows.Close()

	for _, chunk := range pendingChunks {

		embedding, err := generateEmbedding(
			chunk.Text,
			"RETRIEVAL_DOCUMENT",
		)

		if err != nil {
			return err
		}

		embeddingJSON, err := json.Marshal(embedding)
		if err != nil {
			return err
		}

		_, err = db.Exec(`
            UPDATE document_chunks
            SET embedding = ?
            WHERE id = ?
        `, string(embeddingJSON), chunk.ID)

		if err != nil {
			return err
		}

		log.Printf("Embedding saved for chunk ID: %d", chunk.ID)
	}

	return nil
}

func cosineSimilarity(a, b []float64) float64 {

	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct float64
	var magnitudeA float64
	var magnitudeB float64

	for i := 0; i < len(a); i++ {

		dotProduct += a[i] * b[i]

		magnitudeA += a[i] * a[i]

		magnitudeB += b[i] * b[i]
	}

	if magnitudeA == 0 || magnitudeB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(magnitudeA) * math.Sqrt(magnitudeB))
}

func searchRelevantChunks(question string, limit int) ([]RetrievedChunk, error) {

	// Generate embedding for user's question
	queryEmbedding, err := generateEmbedding(
		question,
		"RETRIEVAL_QUERY",
	)

	if err != nil {
		return nil, err
	}

	// Retrieve stored document embeddings
	rows, err := db.Query(`
        SELECT id, document_id, chunk_text, embedding
        FROM document_chunks
        WHERE embedding IS NOT NULL AND embedding != ''
    `)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RetrievedChunk

	for rows.Next() {

		var id int
		var documentID string
		var chunkText string
		var embeddingJSON string

		err := rows.Scan(
			&id,
			&documentID,
			&chunkText,
			&embeddingJSON,
		)

		if err != nil {
			return nil, err
		}

		var storedEmbedding []float64

		err = json.Unmarshal(
			[]byte(embeddingJSON),
			&storedEmbedding,
		)

		if err != nil {
			return nil, err
		}

		// Calculate similarity
		score := cosineSimilarity(
			queryEmbedding,
			storedEmbedding,
		)

		results = append(results, RetrievedChunk{
			ID:         id,
			DocumentID: documentID,
			ChunkText:  chunkText,
			Similarity: score,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort from highest similarity to lowest
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	// Return only the top matching chunks
	if limit <= 0 {
		limit = 3
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func chatHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	var req ChatRequest

	err := json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		http.Error(w, "Invalid Json request", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "Message cannot be empty", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if req.ChatID == "" {

		req.ChatID = fmt.Sprintf("%d", time.Now().UnixNano())

		_, err = db.Exec(
			"INSERT INTO chats (id, created_at) VALUES (?, ?)",
			req.ChatID,
			now,
		)

		if err != nil {
			http.Error(w, "Could not create chat session", http.StatusInternalServerError)
			return
		}

	} else {
		var existingID string

		err = db.QueryRow(
			"SELECT id FROM chats WHERE id = ?",
			req.ChatID,
		).Scan(&existingID)

		if err == sql.ErrNoRows {
			http.Error(w, "Chat session not found", http.StatusNotFound)
			return
		}

		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	}

	userMessage := ChatMessage{
		ChatID:    req.ChatID,
		Role:      "user",
		Content:   req.Message,
		CreatedAt: now,
	}

	result, err := db.Exec(
		"INSERT INTO messages (chat_id, role, content, created_at) VALUES (?, ?, ?, ?)",
		userMessage.ChatID,
		userMessage.Role,
		userMessage.Content,
		userMessage.CreatedAt,
	)

	if err != nil {
		http.Error(w, "Could not save user message", http.StatusInternalServerError)
		return
	}

	userMessage.ID, _ = result.LastInsertId()

	relevantChunks, err := searchChunksThroughMCP(req.Message, 3)

	if err != nil {
		log.Println("MCP document search error:", err)
		http.Error(w, "Could not search documents through MCP", http.StatusBadGateway)
		return
	}

	if err != nil {
		log.Println("Document search error:", err)
		http.Error(w, "Could not search documents", http.StatusBadGateway)
		return
	}

	var contextBuilder strings.Builder

	for _, chunk := range relevantChunks {

		fmt.Fprintf(
			&contextBuilder,
			"\nDocument ID: %s\nChunk ID: %d\nContent: %s\n",
			chunk.DocumentID,
			chunk.ID,
			chunk.ChunkText,
		)
	}

	prompt := `
You are an AI Knowledge Assistant.

Answer the user's question using the provided document context.

If the answer is not available in the context, clearly say that the uploaded documents do not contain enough information.

Do not invent information.

DOCUMENT CONTEXT:
` + contextBuilder.String() + `

USER QUESTION:
` + req.Message

	aiResponse, err := generateGeminiResponse(prompt)

	if err != nil {
		log.Println("Gemini API error:", err)
		http.Error(w, "Could not generate AI response", http.StatusBadGateway)
		return
	}

	assistantMessage := ChatMessage{
		ChatID:    req.ChatID,
		Role:      "assistant",
		Content:   aiResponse,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	result, err = db.Exec(
		"INSERT INTO messages (chat_id, role, content, created_at) VALUES (?, ?, ?, ?)",
		assistantMessage.ChatID,
		assistantMessage.Role,
		assistantMessage.Content,
		assistantMessage.CreatedAt,
	)

	if err != nil {
		http.Error(w, "Could not save assistant message", http.StatusInternalServerError)
		return
	}

	assistantMessage.ID, _ = result.LastInsertId()

	sources := make([]map[string]interface{}, 0)

	for _, chunk := range relevantChunks {

		sources = append(sources, map[string]interface{}{
			"document_id": chunk.DocumentID,
			"chunk_id":    chunk.ID,
			"similarity":  chunk.Similarity,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"chat_id":           req.ChatID,
		"user_message":      userMessage,
		"assistant_message": assistantMessage,
		"sources":           sources,
	})
}

func chatHistory(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	chatID := r.PathValue("id")

	var existingID string

	err := db.QueryRow(
		"SELECT id FROM chats WHERE id = ?",
		chatID,
	).Scan(&existingID)

	if err == sql.ErrNoRows {
		http.Error(w, "Chat session not found", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	rows, err := db.Query(
		"SELECT id, chat_id, role, content, created_at FROM messages WHERE chat_id = ? ORDER BY id",
		chatID,
	)

	if err != nil {
		http.Error(w, "Could not retrieve chat history", http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	messages := make([]ChatMessage, 0)

	for rows.Next() {
		var msg ChatMessage

		err := rows.Scan(
			&msg.ID,
			&msg.ChatID,
			&msg.Role,
			&msg.Content,
			&msg.CreatedAt,
		)

		if err != nil {
			http.Error(w, "Could not read messages", http.StatusInternalServerError)
			return
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error reading chat history", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(messages)
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	log.Println("Environment variables loaded successfully")

	apikey := os.Getenv("GEMINI_API_KEY")

	if apikey == "" {
		log.Fatal("GEMINI_API_KEY is missing from .env")
	}

	log.Println("Gemini API key loaded successfully")

	initDB()
	defer db.Close()

	// Start MCP client when the "client" argument is provided.
	if len(os.Args) > 1 && os.Args[1] == "client" {
		log.Println("Starting MCP client...")

		if err := runMCPClient(); err != nil {
			log.Fatal("MCP client error:", err)
		}

		return
	}

	// Start MCP server when the "mcp" argument is provided.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		log.Println("Starting MCP server...")
		startMCPServer()
		return
	}

	if err := processPendingEmbeddings(); err != nil {
		log.Println("Error generating document embeddings:", err)
	} else {
		log.Println("Pending document embeddings processed successfully")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/documents", listDocuments)
	mux.HandleFunc("POST /api/documents/upload", uploadDocument)
	mux.HandleFunc("GET /api/documents/{id}/chunks", getDocumentChunks)
	mux.HandleFunc("DELETE /api/documents/{id}", deleteDocument)
	mux.HandleFunc("POST /api/chat", chatHandler)
	mux.HandleFunc("GET /api/chat/history/{id}", chatHistory)

	log.Println("Server running at http:localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", mux))
}
