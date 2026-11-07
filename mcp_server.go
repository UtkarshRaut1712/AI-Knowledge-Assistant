package main

import (
	"context"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Output structure for list_documents
type ListDocumentsOutput struct {
	Documents []Document `json:"documents"`
}

// Output structure for search_knowledge_base
type SearchKnowledgeBaseOutput struct {
	Results []RetrievedChunk `json:"results"`
}

func startMCPServer() {

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "ai-knowledge-assistant",
			Version: "1.0.0",
		},
		nil,
	)

	// Register the list_documents tool.
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "list_documents",
			Description: "List all documents uploaded to the knowledge assistant",
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
			input struct{},
		) (*mcp.CallToolResult, ListDocumentsOutput, error) {

			rows, err := db.Query(
				`SELECT id, name, status FROM documents`,
			)
			if err != nil {
				return nil, ListDocumentsOutput{}, err
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
					return nil, ListDocumentsOutput{}, err
				}

				documents = append(documents, doc)
			}

			if err := rows.Err(); err != nil {
				return nil, ListDocumentsOutput{}, err
			}

			return nil, ListDocumentsOutput{
				Documents: documents,
			}, nil
		},
	)

	// Register the search_knowledge_base tool.
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "search_knowledge_base",
			Description: "Search uploaded documents using semantic similarity",
		},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
			input struct {
				Query      string `json:"query"`
				Limit      int    `json:"limit,omitempty"`
				DocumentID string `json:"document_id"`
			},
		) (*mcp.CallToolResult, SearchKnowledgeBaseOutput, error) {

			if input.Query == "" {
				return nil, SearchKnowledgeBaseOutput{}, fmt.Errorf("query cannot be empty")
			}

			limit := input.Limit

			if limit <= 0 || limit > 10 {
				limit = 3
			}

			results, err := searchRelevantChunks(
				input.Query,
				limit,
				input.DocumentID,
			)

			for _, result := range results {
				log.Printf(
					"MCP SEARCH RESULT | Document ID: %s | Chunk ID: %d | Similarity: %.4f",
					result.DocumentID,
					result.ID,
					result.Similarity,
				)
			}

			if err != nil {
				return nil, SearchKnowledgeBaseOutput{}, err
			}

			return nil, SearchKnowledgeBaseOutput{
				Results: results,
			}, nil
		},
	)

	// Start the MCP server.
	if err := server.Run(
		context.Background(),
		&mcp.StdioTransport{},
	); err != nil {
		log.Printf("MCP server error: %v", err)
	}
}
