package indexer

import (
	"context"
	"os"
	"unicode/utf8"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
)

// ChunkFileWithTreeSitter reads a file, parses it using tree-sitter, and extracts top-level declarations as chunks.
func ChunkFileWithTreeSitter(filePath string) ([]Chunk, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Skip non UTF-8 files to avoid embedding binaries
	if !utf8.Valid(content) {
		return nil, nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}

	query, err := sitter.NewQuery([]byte(`
		(function_declaration) @func
		(method_declaration) @method
		(type_declaration) @type
	`), golang.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(query, tree.RootNode())

	var chunks []Chunk
	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, c := range m.Captures {
			node := c.Node
			chunks = append(chunks, Chunk{
				FilePath:  filePath,
				Content:   node.Content(content),
				StartLine: int(node.StartPoint().Row + 1),
				EndLine:   int(node.EndPoint().Row + 1),
			})
		}
	}

	return chunks, nil
}
