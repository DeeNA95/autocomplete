package completer

import (
	"autocomplete/backend/internal/log"
	"fmt"
)

// EmbedderWithDimensions extends the Embedder interface to include dimension information
type EmbedderWithDimensions interface {
	Embedder
	GetDimensions() int
}

// EmbedderFactory creates embedders based on configuration
type EmbedderFactory struct {
	config *Config
}

// NewEmbedderFactory creates a new embedder factory
func NewEmbedderFactory(config *Config) *EmbedderFactory {
	return &EmbedderFactory{
		config: config,
	}
}

// CreateEmbedder creates an embedder based on the configuration
func (f *EmbedderFactory) CreateEmbedder() (EmbedderWithDimensions, error) {
	switch f.config.Embedding.Provider {
	case ProviderOpenAI:
		return f.createOpenAIEmbedder()
	case ProviderLocal:
		return f.createLocalEmbedder()
	case ProviderHuggingFace:
		return f.createHuggingFaceEmbedder()
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", f.config.Embedding.Provider)
	}
}

// createOpenAIEmbedder creates an OpenAI embedder
func (f *EmbedderFactory) createOpenAIEmbedder() (EmbedderWithDimensions, error) {
	log.InfoLogger.Printf("🌐 Initializing OpenAI embedder with model: %s", f.config.Embedding.OpenAI.Model)

	openaiClient := NewOpenAIClient(f.config.Embedding.OpenAI.APIKey)

	// Update the model in the client if it's different from the default
	if f.config.Embedding.OpenAI.Model != "text-embedding-3-small" {
		log.InfoLogger.Printf("📝 Using custom OpenAI embedding model: %s", f.config.Embedding.OpenAI.Model)
	}

	wrapper := &OpenAIEmbedderWrapper{
		client: openaiClient,
		config: f.config.Embedding.OpenAI,
	}

	log.InfoLogger.Printf("✅ OpenAI embedder initialized successfully")
	return wrapper, nil
}

// createLocalEmbedder creates a local embedder
func (f *EmbedderFactory) createLocalEmbedder() (EmbedderWithDimensions, error) {
	log.InfoLogger.Printf("🏠 Initializing local embedder")
	log.InfoLogger.Printf("🔗 Server URL: %s", f.config.Embedding.Local.ServerURL)
	log.InfoLogger.Printf("🤖 Server Type: %s", f.config.Embedding.Local.ServerType)
	if f.config.Embedding.Local.ModelName != "" {
		log.InfoLogger.Printf("📊 Model: %s", f.config.Embedding.Local.ModelName)
	}

	localEmbedder, err := NewLocalEmbedder(f.config.Embedding.Local)
	if err != nil {
		return nil, fmt.Errorf("failed to create local embedder: %w", err)
	}

	// Update config with detected dimensions
	dimensions := localEmbedder.GetDimensions()
	f.config.SetEmbeddingDimensions(dimensions)

	log.InfoLogger.Printf("✅ Local embedder initialized successfully with %d dimensions", dimensions)
	return localEmbedder, nil
}

// createHuggingFaceEmbedder creates a HuggingFace embedder
func (f *EmbedderFactory) createHuggingFaceEmbedder() (EmbedderWithDimensions, error) {
	log.InfoLogger.Printf("🤗 Initializing HuggingFace embedder")
	log.InfoLogger.Printf("📊 Model ID: %s", f.config.Embedding.HuggingFace.ModelID)
	log.InfoLogger.Printf("📁 Cache Dir: %s", f.config.Embedding.HuggingFace.CacheDir)
	log.InfoLogger.Printf("🖥️  Use GPU: %v", f.config.Embedding.HuggingFace.UseGPU)

	hfEmbedder, err := NewHuggingFaceEmbedder(f.config.Embedding.HuggingFace)
	if err != nil {
		return nil, fmt.Errorf("failed to create HuggingFace embedder: %w", err)
	}

	// Update config with detected dimensions
	dimensions := hfEmbedder.GetDimensions()
	f.config.SetEmbeddingDimensions(dimensions)

	log.InfoLogger.Printf("✅ HuggingFace embedder initialized successfully with %d dimensions", dimensions)
	return hfEmbedder, nil
}

// OpenAIEmbedderWrapper wraps the OpenAI client to implement EmbedderWithDimensions
type OpenAIEmbedderWrapper struct {
	client *OpenAIClient
	config OpenAIConfig
}

// Embed delegates to the OpenAI client
func (w *OpenAIEmbedderWrapper) Embed(text string) ([]float32, error) {
	return w.client.Embed(text)
}

// GetDimensions returns the dimensions for the configured OpenAI model
func (w *OpenAIEmbedderWrapper) GetDimensions() int {
	switch w.config.Model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-ada-002":
		return 1536
	default:
		// Default fallback for unknown models
		return 1536
	}
}

// GetEmbeddingDimensions is a convenience function to get dimensions from config
func GetEmbeddingDimensions(config *Config) int {
	if config.Embedding.Dimensions > 0 {
		return config.Embedding.Dimensions
	}
	return config.GetEmbeddingDimensions()
}

// ValidateEmbedderConnection tests the embedder connection and returns dimensions
func ValidateEmbedderConnection(embedder EmbedderWithDimensions) error {
	log.InfoLogger.Printf("🔍 Validating embedder connection...")

	// Test with a simple embedding
	testText := "Hello world"
	embedding, err := embedder.Embed(testText)
	if err != nil {
		return fmt.Errorf("failed to create test embedding: %w", err)
	}

	expectedDimensions := embedder.GetDimensions()
	actualDimensions := len(embedding)

	if actualDimensions != expectedDimensions {
		return fmt.Errorf("dimension mismatch: expected %d, got %d", expectedDimensions, actualDimensions)
	}

	log.InfoLogger.Printf("✅ Embedder connection validated successfully (%d dimensions)", actualDimensions)
	return nil
}
