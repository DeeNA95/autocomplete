package completer

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EmbeddingProvider represents the type of embedding provider
type EmbeddingProvider string

const (
	ProviderOpenAI      EmbeddingProvider = "openai"
	ProviderLocal       EmbeddingProvider = "local"
	ProviderHuggingFace EmbeddingProvider = "huggingface"
)

// Config holds all configuration for the completion service
type Config struct {
	Embedding EmbeddingConfig `json:"embedding"`

	// New exclusion settings
	ExcludedFiles      []string `json:"excluded_files"`
	ExcludedExtensions []string `json:"excluded_extensions"`
}

// EmbeddingConfig holds configuration for embedding providers
type EmbeddingConfig struct {
	Provider        EmbeddingProvider `json:"provider"`
	OpenAI          OpenAIConfig      `json:"openai"`
	Local           LocalConfig       `json:"local"`
	HuggingFace     HuggingFaceConfig `json:"huggingface"`
	Dimensions      int               `json:"dimensions"`       // Auto-detected if 0
	CompletionModel string            `json:"completion_model"` // For text completion (OpenAI)
}

// OpenAIConfig holds OpenAI-specific configuration
type OpenAIConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

// LocalConfig holds local embedding server configuration
type LocalConfig struct {
	ServerURL  string `json:"server_url"`
	ModelName  string `json:"model_name"`
	Timeout    int    `json:"timeout_seconds"`
	ServerType string `json:"server_type"` // "tei", "ollama", "custom"
}

// HuggingFaceConfig holds HuggingFace model configuration
type HuggingFaceConfig struct {
	ModelID   string `json:"model_id"`
	CacheDir  string `json:"cache_dir"`
	UseGPU    bool   `json:"use_gpu"`
	MaxLength int    `json:"max_length"`
	BatchSize int    `json:"batch_size"`
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		Embedding: EmbeddingConfig{
			Provider: ProviderOpenAI, // Default to OpenAI for backward compatibility
			OpenAI: OpenAIConfig{
				Model: "text-embedding-3-small",
			},
			Local: LocalConfig{
				ServerURL:  "http://localhost:8080",
				Timeout:    30,
				ServerType: "tei",
			},
			HuggingFace: HuggingFaceConfig{
				ModelID:   "sentence-transformers/all-MiniLM-L6-v2",
				CacheDir:  "./models",
				UseGPU:    false,
				MaxLength: 512,
				BatchSize: 1,
			},
			Dimensions: 0, // Auto-detect
		},
	}

	// Load embedding provider type
	if provider := os.Getenv("EMBEDDING_PROVIDER"); provider != "" {
		switch strings.ToLower(provider) {
		case "openai":
			config.Embedding.Provider = ProviderOpenAI
		case "local":
			config.Embedding.Provider = ProviderLocal
		case "huggingface":
			config.Embedding.Provider = ProviderHuggingFace
		default:
			return nil, fmt.Errorf("invalid embedding provider: %s (must be 'openai', 'local', or 'huggingface')", provider)
		}
	}

	// Load OpenAI configuration
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		config.Embedding.OpenAI.APIKey = apiKey
	}
	if model := os.Getenv("OPENAI_EMBEDDING_MODEL"); model != "" {
		config.Embedding.OpenAI.Model = model
	}
	if completionModel := os.Getenv("OPENAI_COMPLETION_MODEL"); completionModel != "" {
		config.Embedding.CompletionModel = completionModel
	}

	// Load local embedding configuration
	if serverURL := os.Getenv("LOCAL_EMBEDDING_URL"); serverURL != "" {
		config.Embedding.Local.ServerURL = serverURL
	}
	if modelName := os.Getenv("LOCAL_EMBEDDING_MODEL"); modelName != "" {
		config.Embedding.Local.ModelName = modelName
	}
	if serverType := os.Getenv("LOCAL_EMBEDDING_SERVER_TYPE"); serverType != "" {
		config.Embedding.Local.ServerType = strings.ToLower(serverType)
	}
	if timeoutStr := os.Getenv("LOCAL_EMBEDDING_TIMEOUT"); timeoutStr != "" {
		if timeout, err := strconv.Atoi(timeoutStr); err == nil && timeout > 0 {
			config.Embedding.Local.Timeout = timeout
		}
	}
	// Set default completion model to empty string if not set (set default during client creation)
	if config.Embedding.CompletionModel == "" {
		config.Embedding.CompletionModel = ""
	}

	// Load HuggingFace configuration
	if modelID := os.Getenv("HUGGINGFACE_MODEL_ID"); modelID != "" {
		config.Embedding.HuggingFace.ModelID = modelID
	}
	if cacheDir := os.Getenv("HUGGINGFACE_CACHE_DIR"); cacheDir != "" {
		config.Embedding.HuggingFace.CacheDir = cacheDir
	}
	if useGPUStr := os.Getenv("HUGGINGFACE_USE_GPU"); useGPUStr != "" {
		if useGPU, err := strconv.ParseBool(useGPUStr); err == nil {
			config.Embedding.HuggingFace.UseGPU = useGPU
		}
	}
	if maxLengthStr := os.Getenv("HUGGINGFACE_MAX_LENGTH"); maxLengthStr != "" {
		if maxLength, err := strconv.Atoi(maxLengthStr); err == nil && maxLength > 0 {
			config.Embedding.HuggingFace.MaxLength = maxLength
		}
	}
	if batchSizeStr := os.Getenv("HUGGINGFACE_BATCH_SIZE"); batchSizeStr != "" {
		if batchSize, err := strconv.Atoi(batchSizeStr); err == nil && batchSize > 0 {
			config.Embedding.HuggingFace.BatchSize = batchSize
		}
	}

	// Load embedding dimensions override
	if dimStr := os.Getenv("EMBEDDING_DIMENSIONS"); dimStr != "" {
		if dimensions, err := strconv.Atoi(dimStr); err == nil && dimensions > 0 {
			config.Embedding.Dimensions = dimensions
		}
	}

	// Load exclusion settings
	if excludedFiles := os.Getenv("EXCLUDED_FILES"); excludedFiles != "" {
		files := strings.Split(excludedFiles, ",")
		for i := range files {
			files[i] = strings.TrimSpace(files[i])
		}
		config.ExcludedFiles = files
	}
	if excludedExtensions := os.Getenv("EXCLUDED_EXTENSIONS"); excludedExtensions != "" {
		exts := strings.Split(excludedExtensions, ",")
		for i := range exts {
			exts[i] = strings.TrimSpace(exts[i])
		}
		config.ExcludedExtensions = exts
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	switch c.Embedding.Provider {
	case ProviderOpenAI:
		if c.Embedding.OpenAI.APIKey == "" {
			return fmt.Errorf("OpenAI API key is required when using OpenAI provider")
		}
		if c.Embedding.OpenAI.Model == "" {
			return fmt.Errorf("OpenAI model is required")
		}
	case ProviderLocal:
		if c.Embedding.Local.ServerURL == "" {
			return fmt.Errorf("local embedding server URL is required when using local provider")
		}
		if c.Embedding.Local.Timeout <= 0 {
			return fmt.Errorf("local embedding timeout must be positive")
		}
		validServerTypes := []string{"tei", "ollama", "custom"}
		isValidType := false
		for _, validType := range validServerTypes {
			if c.Embedding.Local.ServerType == validType {
				isValidType = true
				break
			}
		}
		if !isValidType {
			return fmt.Errorf("invalid server type: %s (must be one of: %s)",
				c.Embedding.Local.ServerType, strings.Join(validServerTypes, ", "))
		}
	case ProviderHuggingFace:
		if c.Embedding.HuggingFace.ModelID == "" {
			return fmt.Errorf("HuggingFace model ID is required when using HuggingFace provider")
		}
		if c.Embedding.HuggingFace.MaxLength <= 0 {
			return fmt.Errorf("HuggingFace max length must be positive")
		}
		if c.Embedding.HuggingFace.BatchSize <= 0 {
			return fmt.Errorf("HuggingFace batch size must be positive")
		}
	default:
		return fmt.Errorf("unknown embedding provider: %s", c.Embedding.Provider)
	}

	if c.Embedding.Dimensions < 0 {
		return fmt.Errorf("embedding dimensions must be non-negative")
	}

	return nil
}

// GetEmbeddingDimensions returns the configured embedding dimensions
// If dimensions is 0 (auto-detect), this should be called after dimension detection
func (c *Config) GetEmbeddingDimensions() int {
	switch c.Embedding.Provider {
	case ProviderOpenAI:
		// Known dimensions for OpenAI models
		switch c.Embedding.OpenAI.Model {
		case "text-embedding-3-small":
			return 1536
		case "text-embedding-3-large":
			return 3072
		case "text-embedding-ada-002":
			return 1536
		default:
			return 1536 // Default fallback
		}
	case ProviderLocal:
		// For local embeddings, dimensions should be auto-detected or manually configured
		if c.Embedding.Dimensions > 0 {
			return c.Embedding.Dimensions
		}
		// If not configured, will be auto-detected during embedder initialization
		return 0
	case ProviderHuggingFace:
		// For HuggingFace embeddings, dimensions should be auto-detected or manually configured
		if c.Embedding.Dimensions > 0 {
			return c.Embedding.Dimensions
		}
		// Common HuggingFace model dimensions (will be auto-detected)
		switch c.Embedding.HuggingFace.ModelID {
		case "BAAI/bge-small-en-v1.5", "sentence-transformers/all-MiniLM-L6-v2":
			return 384
		case "BAAI/bge-base-en-v1.5", "sentence-transformers/all-MiniLM-L12-v2":
			return 768
		case "BAAI/bge-large-en-v1.5":
			return 1024
		default:
			return 0 // Will be auto-detected
		}
	}
	return 1536 // Fallback
}

// SetEmbeddingDimensions sets the embedding dimensions (used after auto-detection)
func (c *Config) SetEmbeddingDimensions(dimensions int) {
	c.Embedding.Dimensions = dimensions
}
