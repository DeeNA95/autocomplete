package completer

import (
	"autocomplete/backend/internal/log"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hupe1980/go-huggingface"
)

// HuggingFaceEmbedder implements the Embedder interface using HuggingFace models
type HuggingFaceEmbedder struct {
	config     HuggingFaceConfig
	client     *huggingface.InferenceClient
	dimensions int
}

// NewHuggingFaceEmbedder creates a new HuggingFace embedder instance
func NewHuggingFaceEmbedder(config HuggingFaceConfig) (*HuggingFaceEmbedder, error) {
	log.InfoLogger.Printf("🤗 Initializing HuggingFace embedder with model: %s", config.ModelID)

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(config.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	log.InfoLogger.Printf("📁 Using cache directory: %s", config.CacheDir)

	// Get API token from environment or use empty string for public models
	token := os.Getenv("HUGGINGFACEHUB_API_TOKEN")
	if token == "" {
		token = os.Getenv("HF_TOKEN")
	}
	if token == "" {
		log.InfoLogger.Printf("⚠️  No HuggingFace API token found. Some models may require authentication.")
		log.InfoLogger.Printf("💡 Set HUGGINGFACEHUB_API_TOKEN or HF_TOKEN environment variable if needed.")
	} else {
		log.InfoLogger.Printf("🔑 Using HuggingFace API token for authentication")
	}

	// Create HuggingFace inference client
	client := huggingface.NewInferenceClient(token)

	// Set the model on the client
	client.SetModel(config.ModelID)

	embedder := &HuggingFaceEmbedder{
		config: config,
		client: client,
	}

	// Auto-detect embedding dimensions
	dimensions, err := embedder.detectDimensions()
	if err != nil {
		return nil, fmt.Errorf("failed to detect embedding dimensions: %w", err)
	}
	embedder.dimensions = dimensions

	log.InfoLogger.Printf("✅ HuggingFace embedder initialized successfully with %d dimensions", dimensions)
	return embedder, nil
}

// Embed creates a vector embedding for the given text using the HuggingFace model
func (e *HuggingFaceEmbedder) Embed(text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	// Truncate text if it exceeds max length
	if len(text) > e.config.MaxLength {
		log.InfoLogger.Printf("⚠️  Truncating text from %d to %d characters", len(text), e.config.MaxLength)
		text = text[:e.config.MaxLength]
	}

	// Create embedding request
	req := &huggingface.FeatureExtractionRequest{
		Inputs: []string{text},
		Options: huggingface.Options{
			WaitForModel: huggingface.PTR(true),
			UseCache:     huggingface.PTR(true),
		},
	}

	log.InfoLogger.Printf("🔄 Creating embedding for text (length: %d chars)", len(text))

	// Get embeddings from HuggingFace using automatic reduction to get [][]float32
	resp, err := e.client.FeatureExtractionWithAutomaticReduction(context.Background(), req)
	if err != nil {
		// Check for authentication errors and provide helpful guidance
		if strings.Contains(err.Error(), "Invalid username or password") ||
			strings.Contains(err.Error(), "unauthorized") ||
			strings.Contains(err.Error(), "authentication") {
			return nil, fmt.Errorf("authentication failed for model %s. This model may require a HuggingFace API token. Please set the HUGGINGFACEHUB_API_TOKEN environment variable. You can get a token from https://huggingface.co/settings/tokens. Original error: %w", e.config.ModelID, err)
		}
		return nil, fmt.Errorf("failed to get embeddings from HuggingFace model %s: %w", e.config.ModelID, err)
	}

	if len(resp) == 0 {
		return nil, fmt.Errorf("received empty embedding response")
	}

	// Extract the first embedding (since we only sent one text)
	embedding := resp[0]
	if len(embedding) != e.dimensions {
		return nil, fmt.Errorf("dimension mismatch: expected %d, got %d", e.dimensions, len(embedding))
	}

	log.InfoLogger.Printf("✅ Embedding created successfully (%d dimensions)", len(embedding))
	return embedding, nil
}

// GetDimensions returns the embedding dimensions
func (e *HuggingFaceEmbedder) GetDimensions() int {
	return e.dimensions
}

// detectDimensions auto-detects the embedding dimensions by making a test request
func (e *HuggingFaceEmbedder) detectDimensions() (int, error) {
	log.InfoLogger.Printf("🔍 Auto-detecting embedding dimensions for model: %s", e.config.ModelID)

	testText := "Hello world"

	// Create a test embedding request
	req := &huggingface.FeatureExtractionRequest{
		Inputs: []string{testText},
		Options: huggingface.Options{
			WaitForModel: huggingface.PTR(true),
			UseCache:     huggingface.PTR(true),
		},
	}

	// Make test request to get dimensions
	resp, err := e.client.FeatureExtractionWithAutomaticReduction(context.Background(), req)
	if err != nil {
		// Check for authentication errors and provide helpful guidance
		if strings.Contains(err.Error(), "Invalid username or password") ||
			strings.Contains(err.Error(), "unauthorized") ||
			strings.Contains(err.Error(), "authentication") {
			return 0, fmt.Errorf("authentication failed for model %s during dimension detection. This model may require a HuggingFace API token. Please set the HUGGINGFACEHUB_API_TOKEN environment variable. You can get a token from https://huggingface.co/settings/tokens. Original error: %w", e.config.ModelID, err)
		}
		return 0, fmt.Errorf("failed to make test embedding request for model %s: %w", e.config.ModelID, err)
	}

	if len(resp) == 0 || len(resp[0]) == 0 {
		return 0, fmt.Errorf("received empty embedding during dimension detection")
	}

	dimensions := len(resp[0])
	log.InfoLogger.Printf("✅ Detected %d embedding dimensions", dimensions)

	return dimensions, nil
}

// GetModelInfo returns information about the loaded model
func (e *HuggingFaceEmbedder) GetModelInfo() map[string]interface{} {
	return map[string]interface{}{
		"model_id":   e.config.ModelID,
		"cache_dir":  e.config.CacheDir,
		"use_gpu":    e.config.UseGPU,
		"max_length": e.config.MaxLength,
		"batch_size": e.config.BatchSize,
		"dimensions": e.dimensions,
	}
}

// ValidateModel checks if the model exists and is accessible
func (e *HuggingFaceEmbedder) ValidateModel() error {
	log.InfoLogger.Printf("🔍 Validating HuggingFace model: %s", e.config.ModelID)

	// Try to create a simple embedding to validate the model
	testText := "validation test"
	_, err := e.Embed(testText)
	if err != nil {
		return fmt.Errorf("model validation failed: %w", err)
	}

	log.InfoLogger.Printf("✅ Model validation successful")
	return nil
}

// ClearCache removes cached model files
func (e *HuggingFaceEmbedder) ClearCache() error {
	log.InfoLogger.Printf("🧹 Clearing model cache directory: %s", e.config.CacheDir)

	if _, err := os.Stat(e.config.CacheDir); os.IsNotExist(err) {
		log.InfoLogger.Printf("📁 Cache directory doesn't exist, nothing to clear")
		return nil
	}

	// Remove all files in cache directory but keep the directory itself
	entries, err := os.ReadDir(e.config.CacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		path := filepath.Join(e.config.CacheDir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			log.InfoLogger.Printf("⚠️  Failed to remove %s: %v", path, err)
		} else {
			log.InfoLogger.Printf("🗑️  Removed %s", path)
		}
	}

	log.InfoLogger.Printf("✅ Cache cleared successfully")
	return nil
}

// GetCacheSize returns the size of the model cache in bytes
func (e *HuggingFaceEmbedder) GetCacheSize() (int64, error) {
	var size int64

	err := filepath.Walk(e.config.CacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to calculate cache size: %w", err)
	}

	return size, nil
}

// BatchEmbed creates embeddings for multiple texts at once (future enhancement)
func (e *HuggingFaceEmbedder) BatchEmbed(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	log.InfoLogger.Printf("🔄 Creating batch embeddings for %d texts", len(texts))

	// For now, process texts individually
	// TODO: Implement true batch processing when the library supports it better
	results := make([][]float32, len(texts))

	for i, text := range texts {
		embedding, err := e.Embed(text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		results[i] = embedding
	}

	log.InfoLogger.Printf("✅ Batch embeddings created successfully")
	return results, nil
}
