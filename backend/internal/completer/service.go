package completer

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"autocomplete/backend/internal/cache"
	"autocomplete/backend/internal/indexer"
	"autocomplete/backend/internal/log"
	"autocomplete/backend/internal/storage"
)

// CompletionService provides core logic for directory/file indexing,
// embedding generation (with caching), and vector store management.
type CompletionService struct {
	db          storage.VectorStore
	embedder    Embedder
	llm         *OpenAIClient
	cache       cache.EmbeddingCache
	indexedData map[string][]indexer.Chunk

	// Add config to access exclusion settings
	config *Config
}

// NewCompletionService constructs a CompletionService with the given
// vector store, embedder, LLM client, and embedding cache.
func NewCompletionService(
	db storage.VectorStore,
	embedder Embedder,
	llm *OpenAIClient,
	embCache cache.EmbeddingCache,
	config *Config,
) *CompletionService {
	return &CompletionService{
		db:          db,
		embedder:    embedder,
		llm:         llm,
		cache:       embCache,
		indexedData: make(map[string][]indexer.Chunk),
		config:      config,
	}
}

// isExcluded checks if a file name should be ignored during directory indexing,
// factoring in the default static exclusions plus user-configured excludes.
func (s *CompletionService) isExcluded(name string) bool {
	staticExcludes := map[string]bool{
		"package-lock.json": true,
		"yarn.lock":         true,
		"pnpm-lock.yaml":    true,
		"go.sum":            true,
	}
	if staticExcludes[name] {
		return true
	}

	// Default suffix excludes
	defaultSuffixes := []string{".lock", ".csv", ".json", ".svg", ".png", ".a", ".o", ".so"}
	for _, suffix := range defaultSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	// Check user-configured excluded files
	for _, excludedFile := range s.config.ExcludedFiles {
		if excludedFile != "" && excludedFile == name {
			return true
		}
	}

	// Check user-configured excluded extensions (case-insensitive)
	lowerName := strings.ToLower(name)
	for _, excludedExt := range s.config.ExcludedExtensions {
		if excludedExt != "" && strings.HasSuffix(lowerName, "."+strings.ToLower(excludedExt)) {
			return true
		}
	}

	return false
}

// IndexDirectory walks the given root directory, chunks files,
// builds/loads index, and persists it for future runs.
func (s *CompletionService) IndexDirectory(root string) error {
	cacheDir, err := userCacheDirForRoot(root)
	if err != nil {
		log.ErrorLogger.Printf("⚠️ Failed to get cache dir: %v", err)
		// fallback to root
		cacheDir = root
	}
	log.InfoLogger.Printf("🗂 Using cache directory for index: %s", cacheDir)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		log.ErrorLogger.Printf("⚠️ Failed to create cache directory %s: %v", cacheDir, err)
		// fallback to root
		cacheDir = root
	}
	log.InfoLogger.Printf("🗂 Final cache directory path: %s", cacheDir)
	indexFile := filepath.Join(cacheDir, "index.gob")
	log.InfoLogger.Printf("🗂 Index file path: %s", indexFile)
	if _, err := os.Stat(indexFile); err == nil {
		log.InfoLogger.Printf("💾 Index file found, loading: %s", indexFile)
		return s.LoadIndex(indexFile)
	}

	log.InfoLogger.Printf("📂 Starting to index directory: %s", root)
	ignoredDirs := map[string]bool{
		"node_modules": true,
		"vendor":       true,
		"dist":         true,
		"build":        true,
		"__pycache__":  true,
		"venv":         true,
	}

	var allChunks []indexer.Chunk
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				log.InfoLogger.Printf("🙈 Ignoring hidden directory: %s", path)
				return filepath.SkipDir
			}
			log.InfoLogger.Printf("🙈 Ignoring hidden file: %s", path)
			return nil
		}
		if info.IsDir() {
			if ignoredDirs[info.Name()] {
				log.InfoLogger.Printf("🙈 Ignoring directory: %s", path)
				return filepath.SkipDir
			}
		} else {
			if s.isExcluded(info.Name()) {
				log.InfoLogger.Printf("🙈 Ignoring file: %s", path)
				return nil
			}
			log.InfoLogger.Printf("📄 Staging file for indexing: %s", path)
			chunks, err := indexer.ChunkFileWithTreeSitter(path)
			if err != nil {
				log.ErrorLogger.Printf("⚠️ Could not chunk file %s: %v. Skipping.", path, err)
				return nil
			}
			allChunks = append(allChunks, chunks...)
		}
		return nil
	})
	log.InfoLogger.Printf("📝 Found %d chunks to stage for indexing.", len(allChunks))
	for _, chunk := range allChunks {
		s.indexedData[chunk.FilePath] = append(s.indexedData[chunk.FilePath], chunk)
	}

	if err := s.reIndex(); err != nil {
		return err
	}

	if err := s.SaveIndex(indexFile); err != nil {
		log.ErrorLogger.Printf("⚠️ Failed to save index to %s: %v", indexFile, err)
	} else {
		log.InfoLogger.Printf("💾 Index saved to %s", indexFile)
	}

	log.InfoLogger.Printf("✅ Finished indexing directory: %s", root)
	return nil
}

// reIndex rebuilds the vector store index from staged data, using cache.
func (s *CompletionService) reIndex() error {
	log.InfoLogger.Println("🔄 Rebuilding vector index...")
	var allChunks []indexer.Chunk
	for _, list := range s.indexedData {
		allChunks = append(allChunks, list...)
	}
	if len(allChunks) == 0 {
		log.InfoLogger.Println("No data to index.")
		return s.db.Add(nil, nil)
	}

	var embeddings [][]float32
	var documents []string
	for _, chunk := range allChunks {
		key := cache.ComputeKey(chunk.FilePath, chunk.Content)
		var emb []float32
		if cached, found := s.cache.Get(key); found {
			emb = cached
		} else {
			newEmb, err := s.embedder.Embed(chunk.Content)
			if err != nil {
				log.ErrorLogger.Printf("⚠️ Could not create embedding for chunk from %s: %v. Skipping.", chunk.FilePath, err)
				continue
			}
			s.cache.Set(key, newEmb)
			emb = newEmb
		}
		embeddings = append(embeddings, emb)
		documents = append(documents, chunk.Content)
	}

	if len(embeddings) == 0 {
		log.InfoLogger.Println("No embeddings were generated. Nothing to add.")
		return nil
	}

	log.InfoLogger.Printf("💾 Adding %d embeddings to the vector store.", len(embeddings))
	if err := s.db.Add(embeddings, documents); err != nil {
		return fmt.Errorf("failed to add batch: %w", err)
	}
	log.InfoLogger.Println("✅ Vector index rebuilt.")
	return nil
}

// SaveIndex writes the in-memory index, including cached embeddings, to disk.
func (s *CompletionService) SaveIndex(filePath string) error {
	var allChunks []indexer.Chunk
	for _, list := range s.indexedData {
		allChunks = append(allChunks, list...)
	}

	var embeddings [][]float32
	var documents []string
	for _, chunk := range allChunks {
		key := cache.ComputeKey(chunk.FilePath, chunk.Content)
		var emb []float32
		if cached, found := s.cache.Get(key); found {
			emb = cached
		} else {
			newEmb, err := s.embedder.Embed(chunk.Content)
			if err != nil {
				log.ErrorLogger.Printf("⚠️ Could not create embedding for chunk from %s: %v. Skipping.", chunk.FilePath, err)
				continue
			}
			s.cache.Set(key, newEmb)
			emb = newEmb
		}
		embeddings = append(embeddings, emb)
		documents = append(documents, chunk.Content)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	payload := struct {
		IndexedData map[string][]indexer.Chunk
		Embeddings  [][]float32
		Documents   []string
	}{
		IndexedData: s.indexedData,
		Embeddings:  embeddings,
		Documents:   documents,
	}
	return enc.Encode(&payload)
}

// LoadIndex restores staged chunks and vector store from a saved index file.
func (s *CompletionService) LoadIndex(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	payload := struct {
		IndexedData map[string][]indexer.Chunk
		Embeddings  [][]float32
		Documents   []string
	}{}
	if err := gob.NewDecoder(f).Decode(&payload); err != nil {
		return err
	}

	s.indexedData = payload.IndexedData
	if err := s.db.Add(payload.Embeddings, payload.Documents); err != nil {
		return err
	}
	log.InfoLogger.Printf("✅ Index loaded from %s with %d documents.", filePath, len(payload.Documents))
	return nil
}

// IndexFile stages and re-indexes a single file path.
func (s *CompletionService) IndexFile(path string) error {
	log.InfoLogger.Printf("📄 Indexing single file: %s", path)
	delete(s.indexedData, path)
	chunks, err := indexer.ChunkFileWithTreeSitter(path)
	if err != nil {
		return fmt.Errorf("could not chunk file %s: %w", path, err)
	}
	s.indexedData[path] = chunks
	log.InfoLogger.Printf("📝 Staged %d chunks from %s", len(chunks), path)
	return s.reIndex()
}

// DeleteFile removes a file's staged chunks and re-builds the index.
func (s *CompletionService) DeleteFile(path string) error {
	log.InfoLogger.Printf("🗑️ Deleting file from index: %s", path)
	if _, ok := s.indexedData[path]; !ok {
		log.InfoLogger.Printf("Nothing to delete for %s", path)
		return nil
	}
	delete(s.indexedData, path)
	return s.reIndex()
}

// GetCompletion generates a code completion by embedding the query,
// querying the vector store, building a prompt, and calling the LLM.
func (s *CompletionService) GetCompletion(filePath, content string) (string, error) {
	queryEmb, err := s.embedder.Embed(content)
	if err != nil {
		return "", fmt.Errorf("failed to embed query: %w", err)
	}
	similarDocs, err := s.db.Query(queryEmb, 5)
	if err != nil {
		return "", fmt.Errorf("failed to query vector store: %w", err)
	}
	prompt := s.buildPrompt(content, similarDocs)
	return s.llm.Complete(prompt)
}

// GetCompletionStream streams token-by-token completions to the channel.
func (s *CompletionService) GetCompletionStream(filePath, content string, ch chan<- string) {
	queryEmb, err := s.embedder.Embed(content)
	if err != nil {
		log.ErrorLogger.Printf("failed to embed query for streaming: %v", err)
		close(ch)
		return
	}
	similarDocs, err := s.db.Query(queryEmb, 5)
	if err != nil {
		log.ErrorLogger.Printf("failed to query vector store for streaming: %v", err)
		close(ch)
		return
	}
	log.InfoLogger.Printf("Found %d similar documents.", len(similarDocs))
	prompt := s.buildPrompt(content, similarDocs)
	s.llm.GetCompletionStream(prompt, ch)
}

// buildPrompt constructs the LLM prompt including context and rules.
func (s *CompletionService) buildPrompt(currentCode string, contextDocs []string) string {
	context := strings.Join(contextDocs, "\n\n")

	// Detect language from common patterns to provide language-specific guidance
	language := s.detectLanguage(currentCode)

	return fmt.Sprintf(`You are an expert programmer. Complete the incomplete code at the cursor position.

CRITICAL RULES:
1. NEVER repeat already written code - provide ONLY the continuation from cursor position
2. Maintain exact indentation and formatting style of existing code
3. Complete logically - finish current statement/expression before starting new ones
4. Stop at natural breakpoints - don't over-complete beyond immediate need
5. Use variables/functions visible in the current scope and context
6. Return raw code only - no explanations, markdown, or comments

LANGUAGE-SPECIFIC RULES (%s):
%s

CONTINUATION EXAMPLES:
-- "def calculate" → "(param1, param2):" (not "def calculate")
-- "if x ==" → " 5:" (not "if x ==")
-- "myList.app" → "end(item)" (complete method call)
-- "import " → "os" or "sys" (based on context)
-- Partial variable: "user_na" → "me" (complete identifier)

COMPLETION SCOPE GUIDANCE:
-- For partial identifiers: complete the identifier only
-- For partial statements: complete the current statement
-- For structural elements (functions/classes): provide signature + minimal body
-- For control flow: provide condition/header + first line of body
-- Stop after completing the immediate logical unit

INDENTATION RULES:
-- Match existing indentation exactly (spaces vs tabs, amount)
-- For new blocks: increase indentation by one level from parent
-- For continued lines: align with opening delimiter or use hanging indent

CONTEXT FROM SIMILAR CODE:
%s

INCOMPLETE CODE (cursor at end):
%s

CONTINUATION:`, language, s.getLanguageSpecificRules(language), context, currentCode)
}

// detectLanguage uses simple patterns to guess code language.
func (s *CompletionService) detectLanguage(code string) string {
	lower := strings.ToLower(code)
	switch {
	case strings.Contains(lower, "func ") ||
		strings.Contains(lower, "package ") ||
		strings.Contains(lower, ":="):
		return "Go"
	case strings.Contains(lower, "def ") ||
		strings.Contains(lower, "import ") ||
		strings.Contains(lower, "elif "):
		return "Python"
	case strings.Contains(lower, "function ") ||
		strings.Contains(lower, "const ") ||
		strings.Contains(lower, "=>"):
		return "JavaScript"
	case strings.Contains(lower, "public class") ||
		strings.Contains(lower, "public static"):
		return "Java"
	case strings.Contains(lower, "#include") ||
		strings.Contains(lower, "std::"):
		return "C/C++"
	default:
		return "Unknown"
	}
}

// getLanguageSpecificRules returns rules based on detected language.
func (s *CompletionService) getLanguageSpecificRules(lang string) string {
	switch lang {
	case "Go":
		return `- Use camelCase for functions, PascalCase for types
- Handle errors: if err != nil { return err }`
	case "Python":
		return `- Use snake_case for names
- Indent with 4 spaces`
	case "JavaScript":
		return `- Use camelCase, prefer const, complete arrow functions`
	case "Java":
		return `- Use PascalCase for classes, include access modifiers`
	case "C/C++":
		return `- Include headers, manage semicolons and pointers`
	default:
		return `- Follow existing style`
	}
}

// userCacheDirForRoot returns a unique cache directory path for the given workspace root path.
// It uses OS user cache directory with a subfolder named 'autocomplete' and a hash of the root path.
func userCacheDirForRoot(root string) (string, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	hashedRoot := hashStringToHex(root)
	cacheDir := filepath.Join(userCacheDir, "autocomplete", hashedRoot)
	return cacheDir, nil
}

// hashStringToHex computes the SHA256 hash of the input string and returns hex encoding.
func hashStringToHex(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}
