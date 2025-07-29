package completer

// Embedder is an interface for creating vector embeddings from text.
// This will allow us to easily swap between different embedding providers
// like OpenAI, Google, or local models.
type Embedder interface {
	// Embed takes a string of text and returns its vector embedding.
	Embed(text string) ([]float32, error)
}
