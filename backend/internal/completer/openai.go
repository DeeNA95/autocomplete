package completer

import (
	"autocomplete/backend/internal/log"
	"context"
	"errors"
	"io"

	"github.com/sashabaranov/go-openai"
)

// Default embedding model - can be overridden via configuration
const DefaultEmbeddingModel openai.EmbeddingModel = "text-embedding-3-small"

// OpenAIClient is a client for interacting with the OpenAI API.
// It implements the Embedder interface.
type OpenAIClient struct {
	client         *openai.Client
	embeddingModel openai.EmbeddingModel
}

// NewOpenAIClient creates a new OpenAI client with default embedding model.
func NewOpenAIClient(apiKey string) *OpenAIClient {
	return &OpenAIClient{
		client:         openai.NewClient(apiKey),
		embeddingModel: DefaultEmbeddingModel,
	}
}

// NewOpenAIClientWithModel creates a new OpenAI client with a specific embedding model.
func NewOpenAIClientWithModel(apiKey string, model string) *OpenAIClient {
	return &OpenAIClient{
		client:         openai.NewClient(apiKey),
		embeddingModel: openai.EmbeddingModel(model),
	}
}

// Embed creates a vector embedding for the given text using the specified model.
func (c *OpenAIClient) Embed(text string) ([]float32, error) {
	resp, err := c.client.CreateEmbeddings(
		context.Background(),
		openai.EmbeddingRequest{
			Input: []string{text},
			Model: c.embeddingModel,
		},
	)
	if err != nil {
		return nil, err
	}
	return resp.Data[0].Embedding, nil
}

// Complete generates a code completion for the given prompt.
func (c *OpenAIClient) Complete(prompt string) (string, error) {
	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: "gpt-4.1-nano",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

// GetCompletionStream generates a code completion for the given prompt and streams the response.
func (c *OpenAIClient) GetCompletionStream(prompt string, ch chan<- string) {
	defer close(ch)

	req := openai.ChatCompletionRequest{
		Model: "gpt-4.1-nano",
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Stream: true,
	}

	stream, err := c.client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		log.ErrorLogger.Printf("CreateChatCompletionStream error: %v", err)
		return
	}
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			log.InfoLogger.Println("Stream finished.")
			return
		}

		if err != nil {
			log.ErrorLogger.Printf("Stream error: %v", err)
			return
		}

		if len(response.Choices) > 0 {
			log.InfoLogger.Printf("Received chunk: %s", response.Choices[0].Delta.Content)
			ch <- response.Choices[0].Delta.Content
		}
	}
}
