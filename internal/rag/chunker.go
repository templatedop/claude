// Package rag provides retrieval-augmented generation capabilities.
package rag

import (
	"regexp"
	"strings"
	"unicode"
)

// ChunkingStrategy represents the strategy for splitting documents.
type ChunkingStrategy string

const (
	ChunkingStrategyFixed     ChunkingStrategy = "fixed"     // Fixed size chunks
	ChunkingStrategySentence  ChunkingStrategy = "sentence"  // Sentence-based
	ChunkingStrategyParagraph ChunkingStrategy = "paragraph" // Paragraph-based
	ChunkingStrategySemantic  ChunkingStrategy = "semantic"  // Semantic sections
	ChunkingStrategyCode      ChunkingStrategy = "code"      // Code-aware chunking
	ChunkingStrategyMarkdown  ChunkingStrategy = "markdown"  // Markdown-aware chunking
)

// ChunkOptions contains options for chunking documents.
type ChunkOptions struct {
	Strategy      ChunkingStrategy
	ChunkSize     int     // Target chunk size in characters
	ChunkOverlap  int     // Overlap between chunks
	MinChunkSize  int     // Minimum chunk size (avoid tiny chunks)
	MaxChunkSize  int     // Maximum chunk size (hard limit)
	Separators    []string // Custom separators for splitting
	KeepSeparator bool    // Whether to keep separators in chunks
}

// Chunk represents a document chunk.
type Chunk struct {
	ID         string            `json:"id"`
	Content    string            `json:"content"`
	Index      int               `json:"index"`
	StartChar  int               `json:"start_char"`
	EndChar    int               `json:"end_char"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Importance float32           `json:"importance"` // 0-1 importance weight
}

// DefaultChunkOptions returns default chunking options.
func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{
		Strategy:      ChunkingStrategySentence,
		ChunkSize:     1000,
		ChunkOverlap:  200,
		MinChunkSize:  100,
		MaxChunkSize:  2000,
		Separators:    []string{"\n\n", "\n", ". ", "! ", "? ", "; ", ", ", " "},
		KeepSeparator: true,
	}
}

// Chunker splits documents into chunks.
type Chunker struct {
	opts ChunkOptions
}

// NewChunker creates a new chunker.
func NewChunker(opts ChunkOptions) *Chunker {
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 1000
	}
	if opts.MaxChunkSize <= 0 {
		opts.MaxChunkSize = opts.ChunkSize * 2
	}
	return &Chunker{opts: opts}
}

// ChunkDocument splits a document into chunks.
func (c *Chunker) ChunkDocument(content string, docID string) []Chunk {
	switch c.opts.Strategy {
	case ChunkingStrategyFixed:
		return c.chunkFixed(content, docID)
	case ChunkingStrategySentence:
		return c.chunkBySentence(content, docID)
	case ChunkingStrategyParagraph:
		return c.chunkByParagraph(content, docID)
	case ChunkingStrategyCode:
		return c.chunkCode(content, docID)
	case ChunkingStrategyMarkdown:
		return c.chunkMarkdown(content, docID)
	case ChunkingStrategySemantic:
		return c.chunkSemantic(content, docID)
	default:
		return c.chunkBySentence(content, docID)
	}
}

// chunkFixed creates fixed-size chunks with overlap.
func (c *Chunker) chunkFixed(content string, docID string) []Chunk {
	var chunks []Chunk
	runes := []rune(content)
	length := len(runes)

	if length == 0 {
		return chunks
	}

	step := c.opts.ChunkSize - c.opts.ChunkOverlap
	if step <= 0 {
		step = c.opts.ChunkSize
	}

	for start := 0; start < length; start += step {
		end := start + c.opts.ChunkSize
		if end > length {
			end = length
		}

		chunkContent := string(runes[start:end])
		if len(strings.TrimSpace(chunkContent)) < c.opts.MinChunkSize {
			continue
		}

		chunks = append(chunks, Chunk{
			ID:        generateChunkID(docID, len(chunks)),
			Content:   chunkContent,
			Index:     len(chunks),
			StartChar: start,
			EndChar:   end,
		})

		if end >= length {
			break
		}
	}

	return chunks
}

// chunkBySentence splits by sentences while respecting chunk size.
func (c *Chunker) chunkBySentence(content string, docID string) []Chunk {
	sentences := splitSentences(content)
	return c.mergeUnits(sentences, docID)
}

// chunkByParagraph splits by paragraphs while respecting chunk size.
func (c *Chunker) chunkByParagraph(content string, docID string) []Chunk {
	paragraphs := splitParagraphs(content)
	return c.mergeUnits(paragraphs, docID)
}

// chunkCode splits code into logical blocks.
func (c *Chunker) chunkCode(content string, docID string) []Chunk {
	// Split by function/class definitions and major code blocks
	codeBlocks := splitCodeBlocks(content)
	return c.mergeUnits(codeBlocks, docID)
}

// chunkMarkdown splits markdown by headers.
func (c *Chunker) chunkMarkdown(content string, docID string) []Chunk {
	sections := splitMarkdownSections(content)
	return c.mergeUnits(sections, docID)
}

// chunkSemantic attempts to find semantic boundaries.
func (c *Chunker) chunkSemantic(content string, docID string) []Chunk {
	// Use multiple heuristics to find good split points
	sections := splitSemanticSections(content)
	return c.mergeUnits(sections, docID)
}

// mergeUnits merges text units into chunks respecting size limits.
func (c *Chunker) mergeUnits(units []string, docID string) []Chunk {
	var chunks []Chunk
	var currentChunk strings.Builder
	var currentStart int
	charPos := 0

	flushChunk := func() {
		content := currentChunk.String()
		if len(strings.TrimSpace(content)) >= c.opts.MinChunkSize {
			chunks = append(chunks, Chunk{
				ID:        generateChunkID(docID, len(chunks)),
				Content:   content,
				Index:     len(chunks),
				StartChar: currentStart,
				EndChar:   charPos,
			})
		}
		currentChunk.Reset()
		currentStart = charPos
	}

	for _, unit := range units {
		unitLen := len(unit)

		// If adding this unit would exceed max size, flush current chunk
		if currentChunk.Len()+unitLen > c.opts.MaxChunkSize && currentChunk.Len() > 0 {
			flushChunk()
		}

		// If single unit is larger than max, split it
		if unitLen > c.opts.MaxChunkSize {
			if currentChunk.Len() > 0 {
				flushChunk()
			}
			// Split large unit into fixed chunks
			subChunks := (&Chunker{opts: ChunkOptions{
				ChunkSize:    c.opts.ChunkSize,
				ChunkOverlap: c.opts.ChunkOverlap,
				MinChunkSize: c.opts.MinChunkSize,
				MaxChunkSize: c.opts.MaxChunkSize,
			}}).chunkFixed(unit, docID+"-sub")
			for _, sc := range subChunks {
				sc.ID = generateChunkID(docID, len(chunks))
				sc.Index = len(chunks)
				sc.StartChar += charPos
				sc.EndChar += charPos
				chunks = append(chunks, sc)
			}
			charPos += unitLen
			currentStart = charPos
			continue
		}

		// If we've reached target size with good break point, flush
		if currentChunk.Len() >= c.opts.ChunkSize {
			flushChunk()
		}

		currentChunk.WriteString(unit)
		charPos += unitLen
	}

	// Flush remaining content
	if currentChunk.Len() > 0 {
		flushChunk()
	}

	return chunks
}

// Helper functions

func generateChunkID(docID string, index int) string {
	return docID + "-chunk-" + strings.Replace(string(rune('0'+index)), "0", "", -1) + string(rune('0'+index%10))
}

func splitSentences(text string) []string {
	// Simple sentence splitting (can be improved with NLP)
	sentenceEnders := regexp.MustCompile(`([.!?])\s+`)
	parts := sentenceEnders.Split(text, -1)

	var sentences []string
	matches := sentenceEnders.FindAllStringIndex(text, -1)

	lastEnd := 0
	for i, part := range parts {
		if i < len(matches) {
			end := matches[i][1]
			sentences = append(sentences, text[lastEnd:end])
			lastEnd = end
		} else if len(strings.TrimSpace(part)) > 0 {
			sentences = append(sentences, text[lastEnd:])
		}
	}

	if len(sentences) == 0 && len(text) > 0 {
		sentences = []string{text}
	}

	return sentences
}

func splitParagraphs(text string) []string {
	// Split by double newlines
	paragraphs := regexp.MustCompile(`\n\s*\n`).Split(text, -1)
	var result []string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if len(p) > 0 {
			result = append(result, p+"\n\n")
		}
	}
	return result
}

func splitCodeBlocks(text string) []string {
	// Split by function/class definitions
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^func\s+`),           // Go functions
		regexp.MustCompile(`(?m)^def\s+`),            // Python functions
		regexp.MustCompile(`(?m)^class\s+`),          // Class definitions
		regexp.MustCompile(`(?m)^(public|private|protected)\s+(static\s+)?`), // Java/C# methods
		regexp.MustCompile(`(?m)^(const|let|var)\s+\w+\s*=\s*function`),      // JS functions
		regexp.MustCompile(`(?m)^(const|let|var)\s+\w+\s*=\s*\(`),            // Arrow functions
	}

	lines := strings.Split(text, "\n")
	var blocks []string
	var currentBlock strings.Builder

	for _, line := range lines {
		isStart := false
		for _, pattern := range patterns {
			if pattern.MatchString(line) {
				isStart = true
				break
			}
		}

		if isStart && currentBlock.Len() > 0 {
			blocks = append(blocks, currentBlock.String())
			currentBlock.Reset()
		}

		currentBlock.WriteString(line)
		currentBlock.WriteString("\n")
	}

	if currentBlock.Len() > 0 {
		blocks = append(blocks, currentBlock.String())
	}

	return blocks
}

func splitMarkdownSections(text string) []string {
	// Split by markdown headers
	headerPattern := regexp.MustCompile(`(?m)^#{1,6}\s+`)

	lines := strings.Split(text, "\n")
	var sections []string
	var currentSection strings.Builder

	for _, line := range lines {
		if headerPattern.MatchString(line) && currentSection.Len() > 0 {
			sections = append(sections, currentSection.String())
			currentSection.Reset()
		}
		currentSection.WriteString(line)
		currentSection.WriteString("\n")
	}

	if currentSection.Len() > 0 {
		sections = append(sections, currentSection.String())
	}

	return sections
}

func splitSemanticSections(text string) []string {
	// Combine multiple heuristics
	// 1. Look for section breaks (multiple newlines, horizontal rules)
	// 2. Look for list items at same level
	// 3. Look for logical topic changes

	// For now, use paragraph + sentence hybrid
	paragraphs := splitParagraphs(text)
	var sections []string

	for _, p := range paragraphs {
		// If paragraph is short enough, keep as one section
		if len(p) <= 1500 {
			sections = append(sections, p)
		} else {
			// Split long paragraphs by sentences
			sentences := splitSentences(p)
			sections = append(sections, sentences...)
		}
	}

	return sections
}

// CalculateImportance calculates importance score for a chunk.
func CalculateImportance(chunk Chunk, docTitle string, keywords []string) float32 {
	var score float32 = 0.5 // Base score

	content := strings.ToLower(chunk.Content)
	title := strings.ToLower(docTitle)

	// Higher score for chunks containing title words
	titleWords := strings.Fields(title)
	for _, word := range titleWords {
		if len(word) > 3 && strings.Contains(content, word) {
			score += 0.1
		}
	}

	// Higher score for chunks containing keywords
	for _, keyword := range keywords {
		if strings.Contains(content, strings.ToLower(keyword)) {
			score += 0.1
		}
	}

	// Higher score for chunks at document start (usually important)
	if chunk.Index == 0 {
		score += 0.15
	} else if chunk.Index == 1 {
		score += 0.1
	}

	// Penalize very short chunks
	if len(chunk.Content) < 200 {
		score -= 0.1
	}

	// Bonus for chunks with structural elements
	if strings.Contains(chunk.Content, "##") ||
	   strings.Contains(chunk.Content, "**") ||
	   strings.Contains(chunk.Content, "```") {
		score += 0.05
	}

	// Clamp to 0-1
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

// CountTokens estimates token count (rough approximation).
func CountTokens(text string) int {
	// Rough estimate: ~4 characters per token for English
	// This is a simplification; production should use actual tokenizer
	words := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if inWord {
				words++
				inWord = false
			}
		} else {
			inWord = true
		}
	}
	if inWord {
		words++
	}

	// Add extra for punctuation and special tokens
	return int(float64(words) * 1.3)
}
