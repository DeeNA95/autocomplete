package completer

import (
	"autocomplete/backend/internal/log"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LocalEmbedder implements the Embedder interface using a local embedding server
type LocalEmbedder struct {
	config     LocalConfig
	httpClient *http.Client
	dimensions int
}

// NewLocalEmbedder creates a new local embedder instance
func NewLocalEmbedder(config LocalConfig) (*LocalEmbedder, error) {
	embedder := &LocalEmbedder{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}

	// Auto-detect embedding dimensions
	dimensions, err := embedder.detectDimensions()
	if err != nil {
		return nil, fmt.Errorf("failed to detect embedding dimensions: %w", err)
	}
	embedder.dimensions = dimensions

	log.InfoLogger.Printf("🤖 Local embedder initialized with %d dimensions", dimensions)
	return embedder, nil
}

// Embed creates a vector embedding for the given text using the local server
func (e *LocalEmbedder) Embed(text string) ([]float32, error) {
	requestBody, err := e.createRequestBody([]string{text})
	if err != nil {
		return nil, fmt.Errorf("failed to create request body: %w", err)
	}

	resp, err := e.httpClient.Post(e.getEmbedEndpoint(), "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to make request to local embedding server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("local embedding server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	embedding, err := e.parseResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}

	if len(embedding) == 0 {
		return nil, fmt.Errorf("received empty embedding from local server")
	}

	return embedding[0], nil
}

// GetDimensions returns the embedding dimensions
func (e *LocalEmbedder) GetDimensions() int {
	return e.dimensions
}

// detectDimensions auto-detects the embedding dimensions by making a test request
func (e *LocalEmbedder) detectDimensions() (int, error) {
	log.InfoLogger.Printf("🔍 Auto-detecting embedding dimensions for local server at %s", e.config.ServerURL)

	testText := "test"
	embedding, err := e.embedSingle(testText)
	if err != nil {
		return 0, fmt.Errorf("failed to make test embedding request: %w", err)
	}

	dimensions := len(embedding)
	if dimensions == 0 {
		return 0, fmt.Errorf("received empty embedding during dimension detection")
	}

	log.InfoLogger.Printf("✅ Detected %d embedding dimensions", dimensions)
	return dimensions, nil
}

// embedSingle makes a single embedding request (used for dimension detection)
func (e *LocalEmbedder) embedSingle(text string) ([]float32, error) {
	requestBody, err := e.createRequestBody([]string{text})
	if err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Post(e.getEmbedEndpoint(), "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	embeddings, err := e.parseResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("received empty embeddings")
	}

	return embeddings[0], nil
}

// getEmbedEndpoint returns the embedding endpoint URL based on server type
func (e *LocalEmbedder) getEmbedEndpoint() string {
	baseURL := e.config.ServerURL

	switch e.config.ServerType {
	case "tei":
		return baseURL + "/embed"
	case "ollama":
		return baseURL + "/api/embeddings"
	case "custom":
		return baseURL + "/embeddings"
	default:
		// Default to TEI format
		return baseURL + "/embed"
	}
}

// createRequestBody creates the HTTP request body based on server type
func (e *LocalEmbedder) createRequestBody(texts []string) ([]byte, error) {
	switch e.config.ServerType {
	case "tei":
		return e.createTEIRequestBody(texts)
	case "ollama":
		return e.createOllamaRequestBody(texts)
	case "custom":
		return e.createCustomRequestBody(texts)
	default:
		return e.createTEIRequestBody(texts)
	}
}

// createTEIRequestBody creates request body for Text Embeddings Inference server
func (e *LocalEmbedder) createTEIRequestBody(texts []string) ([]byte, error) {
	request := map[string]interface{}{
		"inputs": texts,
	}
	return json.Marshal(request)
}

// createOllamaRequestBody creates request body for Ollama server
func (e *LocalEmbedder) createOllamaRequestBody(texts []string) ([]byte, error) {
	if len(texts) != 1 {
		return nil, fmt.Errorf("ollama only supports single text embedding")
	}

	request := map[string]interface{}{
		"model":  e.config.ModelName,
		"prompt": texts[0],
	}
	return json.Marshal(request)
}

// createCustomRequestBody creates request body for custom server format
func (e *LocalEmbedder) createCustomRequestBody(texts []string) ([]byte, error) {
	request := map[string]interface{}{
		"input": texts,
	}
	if e.config.ModelName != "" {
		request["model"] = e.config.ModelName
	}
	return json.Marshal(request)
}

// parseResponse parses the embedding response based on server type
func (e *LocalEmbedder) parseResponse(body io.Reader) ([][]float32, error) {
	switch e.config.ServerType {
	case "tei":
		return e.parseTEIResponse(body)
	case "ollama":
		return e.parseOllamaResponse(body)
	case "custom":
		return e.parseCustomResponse(body)
	default:
		return e.parseTEIResponse(body)
	}
}

// parseTEIResponse parses Text Embeddings Inference server response
func (e *LocalEmbedder) parseTEIResponse(body io.Reader) ([][]float32, error) {
	var embeddings [][]float32
	if err := json.NewDecoder(body).Decode(&embeddings); err != nil {
		return nil, fmt.Errorf("failed to decode TEI response: %w", err)
	}
	return embeddings, nil
}

// parseOllamaResponse parses Ollama server response
func (e *LocalEmbedder) parseOllamaResponse(body io.Reader) ([][]float32, error) {
	var response struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama response: %w", err)
	}
	return [][]float32{response.Embedding}, nil
}

// parseCustomResponse parses custom server response (OpenAI-like format)
func (e *LocalEmbedder) parseCustomResponse(body io.Reader) ([][]float32, error) {
	var response struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Embeddings [][]float32 `json:"embeddings"` // Alternative format
	}

	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode custom response: %w", err)
	}

	// Try OpenAI-like format first
	if len(response.Data) > 0 {
		embeddings := make([][]float32, len(response.Data))
		for i, item := range response.Data {
			embeddings[i] = item.Embedding
		}
		return embeddings, nil
	}

	// Try direct embeddings format
	if len(response.Embeddings) > 0 {
		return response.Embeddings, nil
	}

	return nil, fmt.Errorf("no embeddings found in custom response")
}
