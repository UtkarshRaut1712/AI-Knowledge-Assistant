package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func runMCPClient() error {

	// Create a context with a 30-second timeout.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	// Create the MCP client.
	client := mcp.NewClient(
		&mcp.Implementation{
			Name:    "ai-knowledge-assistant-client",
			Version: "1.0.0",
		},
		nil,
	)

	// Start the MCP server as a separate process.
	transport := &mcp.CommandTransport{
		Command: exec.Command("go", "run", ".", "mcp"),
	}

	// Connect the client to the MCP server.
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("MCP connection failed: %w", err)
	}
	defer session.Close()

	log.Println("MCP client connected successfully")

	// Call the list_documents tool.
	result, err := session.CallTool(
		ctx,
		&mcp.CallToolParams{
			Name:      "list_documents",
			Arguments: map[string]any{},
		},
	)

	if err != nil {
		return fmt.Errorf("MCP tool call failed: %w", err)
	}

	if result.IsError {
		return fmt.Errorf("list_documents tool returned an error")
	}

	// Display the tool response.
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	log.Println("MCP tool response:")
	log.Println(string(output))

	// Call the search_knowledge_base MCP tool.
	searchResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_knowledge_base",
		Arguments: map[string]any{
			"query": "What is Golang and how does it support concurrency?",
			"limit": 3,
		},
	})

	if err != nil {
		return fmt.Errorf("MCP search tool call failed: %w", err)
	}

	if searchResult.IsError {
		return fmt.Errorf("search_knowledge_base tool returned an error")
	}

	searchOutput, err := json.MarshalIndent(searchResult, "", "  ")
	if err != nil {
		return err
	}

	log.Println("MCP search tool response:")
	log.Println(string(searchOutput))

	return nil
}

func searchChunksThroughMCP(query string, limit int) ([]RetrievedChunk, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := mcp.NewClient(
		&mcp.Implementation{
			Name:    "ai-knowledge-assistant-client",
			Version: "1.0.0",
		},
		nil,
	)

	cmd := exec.Command("go", "run", ".", "mcp")
	cmd.Stderr = os.Stderr

	transport := &mcp.CommandTransport{
		Command: cmd,
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("MCP connection failed: %w", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_knowledge_base",
		Arguments: map[string]any{
			"query": query,
			"limit": limit,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("MCP search failed: %w", err)
	}

	if result.IsError {
		return nil, fmt.Errorf("MCP search tool returned an error")
	}

	var output SearchKnowledgeBaseOutput

	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &output)
	if err != nil {
		return nil, fmt.Errorf("could not decode MCP search results: %w", err)
	}

	return output.Results, nil
}
