package indexer

import (
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

// Chunk represents a piece of a source code file.
type Chunk struct {
	FilePath  string
	Content   string
	StartLine int
	EndLine   int
}

// ChunkFile reads a file and splits it into chunks based on character size.
// It aims for chunks of 1000 characters with 100 characters of overlap.
// The chunkSize parameter from the function signature is ignored in favor of the constants defined within.
func ChunkFile(filePath string, _ int) ([]Chunk, error) {
	const chunkSize = 1000
	const chunkOverlap = 100

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Skip non UTF-8 files to avoid embedding binaries
	if !utf8.Valid(contentBytes) {
		return nil, nil
	}

	content := string(contentBytes)

	if strings.TrimSpace(content) == "" {
		return nil, nil
	}

	lines := strings.Split(content, "\n")
	lineCharOffsets := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		lineCharOffsets[i] = offset
		offset += len(line) + 1 // +1 for \n
	}

	// findLine returns the 1-based line number for a given character offset.
	findLine := func(charOffset int) int {
		// sort.Search finds the smallest index i where lineCharOffsets[i] > charOffset.
		// This means the character is on the line with index i-1 (0-indexed).
		// The line number is i (1-indexed).
		lineIndex := sort.Search(len(lineCharOffsets), func(i int) bool {
			return lineCharOffsets[i] > charOffset
		})

		if lineIndex == 0 {
			return 1
		}
		return lineIndex
	}

	var chunks []Chunk
	runes := []rune(content)
	for i := 0; i < len(runes); i += chunkSize - chunkOverlap {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		chunkText := string(runes[i:end])
		startLine := findLine(i)

		var endLine int
		if end > 0 {
			endLine = findLine(end - 1)
		} else {
			endLine = startLine
		}

		chunks = append(chunks, Chunk{
			FilePath:  filePath,
			Content:   chunkText,
			StartLine: startLine,
			EndLine:   endLine,
		})

		if end == len(runes) {
			break
		}
	}

	return chunks, nil
}
