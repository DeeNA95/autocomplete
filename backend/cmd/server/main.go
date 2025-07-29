package main

import (
	"autocomplete/backend/internal/cache"
	"autocomplete/backend/internal/completer"
	"autocomplete/backend/internal/log"
	"autocomplete/backend/internal/storage"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// runServerWithAPIKey starts the backend HTTP server using the provided OpenAI API key.
// This function contains the original server setup and request handlers,
// modified to use the injected API key parameter instead of directly reading environment variables.
func runServerWithAPIKey(openaiAPIKey string) {
	router := gin.Default()

	// Load configuration
	config, err := completer.LoadConfig()
	if err != nil {
		log.ErrorLogger.Fatalf("FATAL: Failed to load configuration: %v", err)
	}

	// Create embedder factory and embedder
	factory := completer.NewEmbedderFactory(config)
	embedder, err := factory.CreateEmbedder()
	if err != nil {
		log.ErrorLogger.Fatalf("FATAL: Failed to create embedder: %v", err)
	}

	// Validate embedder connection
	if err := completer.ValidateEmbedderConnection(embedder); err != nil {
		log.ErrorLogger.Fatalf("FATAL: Failed to validate embedder connection: %v", err)
	}

	// Get embedding dimensions and create vector store
	dimensions := embedder.GetDimensions()
	log.InfoLogger.Printf("📏 Using embedding dimensions: %d", dimensions)

	vectorStore, err := storage.NewVectorStore(dimensions)
	if err != nil {
		log.ErrorLogger.Fatalf("Could not create vector store: %v", err)
	}
	defer vectorStore.Close()

	// Use injected OpenAI API key string for completions (still using OpenAI for text generation)
	// Create OpenAI client for completions (still using OpenAI for text generation)
	if openaiAPIKey == "" {
		log.ErrorLogger.Fatalln("FATAL: OpenAI API key must be provided to run the server.")
	}
	completionModel := config.Embedding.CompletionModel
	if completionModel == "" {
		completionModel = "gpt-4.1-nano"
	}
	openaiClient := completer.NewOpenAIClientWithModel(openaiAPIKey, completionModel)

	// Create completion service with configurable embedder but OpenAI for completions
	embCache := cache.NewInMemoryCache()
	completionService := completer.NewCompletionService(vectorStore, embedder, openaiClient, embCache, config)

	// Simple health check endpoint
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Autocomplete backend is running!",
		})
	})

	// Endpoint to trigger workspace indexing
	router.POST("/index", func(c *gin.Context) {
		var jsonBody struct {
			Path string `json:"path"`
		}
		if err := c.ShouldBindJSON(&jsonBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// If no path is provided, use the current working directory.
		path := jsonBody.Path
		if path == "" {
			var err error
			path, err = os.Getwd()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not get working directory"})
				return
			}
		}

		// Run indexing asynchronously
		go func(path string) {
			if err := completionService.IndexDirectory(path); err != nil {
				log.ErrorLogger.Printf("ERROR: Failed to index directory async: %v", err)
			} else {
				log.InfoLogger.Printf("Async indexing completed for directory: %s", path)
			}
		}(path)

		// Immediately respond that indexing started
		c.JSON(http.StatusOK, gin.H{
			"message": "Indexing started for directory: " + path,
		})
	})

	// Endpoint to index a single file
	router.POST("/index-file", func(c *gin.Context) {
		var jsonBody struct {
			Path string `json:"path"`
		}
		if err := c.ShouldBindJSON(&jsonBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Run file indexing asynchronously
		go func(path string) {
			if err := completionService.IndexFile(path); err != nil {
				log.ErrorLogger.Printf("ERROR: Failed to index file async: %v", err)
			} else {
				log.InfoLogger.Printf("Async indexing completed for file: %s", path)
			}
		}(jsonBody.Path)

		// Immediately respond that indexing started
		c.JSON(http.StatusOK, gin.H{
			"message": "Indexing started for file: " + jsonBody.Path,
		})
	})

	// Endpoint to delete a file from the index
	router.DELETE("/index-file", func(c *gin.Context) {
		var jsonBody struct {
			Path string `json:"path"`
		}
		if err := c.ShouldBindJSON(&jsonBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := completionService.DeleteFile(jsonBody.Path); err != nil {
			log.ErrorLogger.Printf("ERROR: Failed to delete file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Deletion completed for file: " + jsonBody.Path})
	})

	// Endpoint to get a code completion
	router.GET("/complete", func(c *gin.Context) {
		filePath := c.Query("file_path")
		content := c.Query("content")

		if filePath == "" || content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file_path and content are required"})
			return
		}

		log.InfoLogger.Printf("Received completion request for file: %s", filePath)

		// Get single completion response
		completion, err := completionService.GetCompletion(filePath, content)
		if err != nil {
			log.ErrorLogger.Printf("Failed to get completion: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate completion"})
			return
		}

		// Return single JSON response
		c.JSON(http.StatusOK, gin.H{"completion": completion})
	})

	log.InfoLogger.Println("🚀 Starting server on http://localhost:2539")
	if err := router.Run(":2539"); err != nil {
		log.ErrorLogger.Fatalf("🔥 Could not start server: %s\n", err)
	}
}

// main is the entry point of the program.
// It reads the injected OpenAI API key from environment variable 'OPENAI_API_KEY_INJECTED'.
// If missing, it fatally fails. Otherwise, starts the server by calling runServerWithAPIKey.
func main() {
	openaiAPIKey := os.Getenv("OPENAI_API_KEY_INJECTED")
	if openaiAPIKey == "" {
		log.ErrorLogger.Fatalln("FATAL: OpenAI API key environment variable 'OPENAI_API_KEY_INJECTED' is not set; ensure it is injected from the VS Code extension.")
	}
	runServerWithAPIKey(openaiAPIKey)
}
