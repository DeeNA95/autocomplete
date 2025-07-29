package cache

import "sync"

// EmbeddingCache defines a simple interface for storing and retrieving embeddings.
type EmbeddingCache interface {
	// Get returns the embedding for the given key and whether it was found.
	Get(key string) ([]float32, bool)
	// Set stores the embedding for the given key.
	Set(key string, embedding []float32)
}

// InMemoryCache is a thread-safe, in-memory implementation of EmbeddingCache.
type InMemoryCache struct {
	mu    sync.RWMutex
	store map[string][]float32
}

// NewInMemoryCache initializes and returns a new in-memory cache.
func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		store: make(map[string][]float32),
	}
}

// Get retrieves an embedding from the cache by key.
// It returns a copy of the stored slice to prevent callers from mutating internal state.
func (c *InMemoryCache) Get(key string) ([]float32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	val, found := c.store[key]
	if !found {
		return nil, false
	}

	// Return a copy to protect against external modifications.
	copied := make([]float32, len(val))
	copy(copied, val)
	return copied, true
}

// Set adds or updates an embedding in the cache.
// It makes an internal copy of the slice to protect against external mutations.
func (c *InMemoryCache) Set(key string, embedding []float32) {
	// Copy the embedding to avoid retaining references to caller's slice.
	copied := make([]float32, len(embedding))
	copy(copied, embedding)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = copied
}
