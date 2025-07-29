package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ComputeKey generates a deterministic cache key for a given file path and content.
// It concatenates the file path and chunk content with a separator, hashes the result
// using SHA-256, and returns the hex-encoded string.
func ComputeKey(filePath, chunkContent string) string {
	// Combine file path and content to form the input for key generation.
	input := fmt.Sprintf("%s:%s", filePath, chunkContent)

	// Compute SHA-256 hash of the combined input.
	sum := sha256.Sum256([]byte(input))

	// Return the hex-encoded representation of the hash.
	return hex.EncodeToString(sum[:])
}
