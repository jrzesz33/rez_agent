package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	DefaultMCPServerURL = "https://your-api-gateway.execute-api.us-east-1.amazonaws.com/mcp"
	DefaultTimeout      = 30 * time.Second
)

func main() {
	// Configure logging (stderr so it doesn't interfere with stdio protocol)
	log.SetOutput(os.Stderr)
	log.SetPrefix("[mcp-client] ")

	// Get configuration from environment
	serverURL := os.Getenv("MCP_SERVER_URL")
	if serverURL == "" {
		serverURL = DefaultMCPServerURL
		log.Printf("Using default MCP_SERVER_URL: %s", serverURL)
	}

	apiKey := os.Getenv("MCP_API_KEY")
	if apiKey == "" {
		log.Printf("Warning: MCP_API_KEY not set")
	}

	log.Printf("MCP Stdio Client starting...")
	log.Printf("Server URL: %s", serverURL)

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: DefaultTimeout,
	}

	// Read JSON-RPC requests from stdin, forward to Lambda, write responses to stdout
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large requests

	for scanner.Scan() {
		requestData := scanner.Bytes()

		if len(requestData) == 0 {
			continue
		}

		log.Printf("Received request (%d bytes)", len(requestData))

		// Forward to Lambda
		responseData, err := forwardToLambda(httpClient, serverURL, apiKey, requestData)
		if err != nil {
			log.Printf("Error forwarding request: %v", err)

			// Send JSON-RPC error response
			errorResp := map[string]interface{}{
				"jsonrpc": "2.0",
				"error": map[string]interface{}{
					"code":    -32603,
					"message": "Communication error with MCP server",
					"data":    err.Error(),
				},
				"id": nil,
			}

			errorData, _ := json.Marshal(errorResp)
			responseData = errorData
		}

		// Write response to stdout
		os.Stdout.Write(responseData)
		os.Stdout.Write([]byte("\n"))
		os.Stdout.Sync()

		log.Printf("Sent response (%d bytes)", len(responseData))
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading stdin: %v", err)
	}

	log.Printf("MCP Stdio Client terminated")
}

// forwardToLambda sends the JSON-RPC request to the Lambda function and returns the response
func forwardToLambda(client *http.Client, url, apiKey string, requestData []byte) ([]byte, error) {
	// Create HTTP request
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(requestData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(responseData))
	}

	return responseData, nil
}
