// Package rag provides retrieval-augmented generation functionality.
package rag

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// Special token IDs for BERT tokenizer
const (
	TokenCLS      = "[CLS]"
	TokenSEP      = "[SEP]"
	TokenPAD      = "[PAD]"
	TokenUNK      = "[UNK]"
	TokenMASK     = "[MASK]"
	SubwordPrefix = "##"
)

// BERTTokenizer implements WordPiece tokenization for BERT-style models.
type BERTTokenizer struct {
	vocab        map[string]int
	reverseVocab map[int]string
	clsID        int
	sepID        int
	padID        int
	unkID        int
	maxLen       int
}

// TokenizerOutput holds the result of tokenization.
type TokenizerOutput struct {
	InputIDs      []int64
	AttentionMask []int64
	TokenCount    int
}

// NewBERTTokenizer creates a new tokenizer from a vocabulary file.
func NewBERTTokenizer(vocabPath string, maxLen int) (*BERTTokenizer, error) {
	data, err := os.ReadFile(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vocab file: %w", err)
	}

	var vocab map[string]int
	if err := json.Unmarshal(data, &vocab); err != nil {
		return nil, fmt.Errorf("failed to parse vocab JSON: %w", err)
	}

	// Build reverse vocab for decoding
	reverseVocab := make(map[int]string, len(vocab))
	for token, id := range vocab {
		reverseVocab[id] = token
	}

	// Get special token IDs
	clsID, ok := vocab[TokenCLS]
	if !ok {
		return nil, fmt.Errorf("vocab missing %s token", TokenCLS)
	}
	sepID, ok := vocab[TokenSEP]
	if !ok {
		return nil, fmt.Errorf("vocab missing %s token", TokenSEP)
	}
	padID, ok := vocab[TokenPAD]
	if !ok {
		return nil, fmt.Errorf("vocab missing %s token", TokenPAD)
	}
	unkID, ok := vocab[TokenUNK]
	if !ok {
		return nil, fmt.Errorf("vocab missing %s token", TokenUNK)
	}

	return &BERTTokenizer{
		vocab:        vocab,
		reverseVocab: reverseVocab,
		clsID:        clsID,
		sepID:        sepID,
		padID:        padID,
		unkID:        unkID,
		maxLen:       maxLen,
	}, nil
}

// Encode tokenizes text and returns padded input IDs and attention mask.
func (t *BERTTokenizer) Encode(text string) *TokenizerOutput {
	// Basic text normalization
	text = t.normalizeText(text)

	// Tokenize into word pieces
	tokens := t.tokenize(text)

	// Truncate if necessary (leave room for [CLS] and [SEP])
	maxTokens := t.maxLen - 2
	if len(tokens) > maxTokens {
		tokens = tokens[:maxTokens]
	}

	// Build input IDs: [CLS] + tokens + [SEP]
	inputIDs := make([]int64, t.maxLen)
	attentionMask := make([]int64, t.maxLen)

	inputIDs[0] = int64(t.clsID)
	attentionMask[0] = 1

	for i, token := range tokens {
		inputIDs[i+1] = int64(t.tokenToID(token))
		attentionMask[i+1] = 1
	}

	sepPos := len(tokens) + 1
	inputIDs[sepPos] = int64(t.sepID)
	attentionMask[sepPos] = 1

	// Fill rest with padding
	for i := sepPos + 1; i < t.maxLen; i++ {
		inputIDs[i] = int64(t.padID)
		attentionMask[i] = 0
	}

	return &TokenizerOutput{
		InputIDs:      inputIDs,
		AttentionMask: attentionMask,
		TokenCount:    len(tokens) + 2, // +2 for [CLS] and [SEP]
	}
}

// EncodeBatch tokenizes multiple texts.
func (t *BERTTokenizer) EncodeBatch(texts []string) []*TokenizerOutput {
	results := make([]*TokenizerOutput, len(texts))
	for i, text := range texts {
		results[i] = t.Encode(text)
	}
	return results
}

// normalizeText performs basic text normalization.
func (t *BERTTokenizer) normalizeText(text string) string {
	// Convert to lowercase (BERT uncased)
	text = strings.ToLower(text)

	// Normalize whitespace
	text = strings.TrimSpace(text)

	// Replace multiple spaces with single space
	var result strings.Builder
	prevSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !prevSpace {
				result.WriteRune(' ')
				prevSpace = true
			}
		} else {
			result.WriteRune(r)
			prevSpace = false
		}
	}

	return result.String()
}

// tokenize performs WordPiece tokenization.
func (t *BERTTokenizer) tokenize(text string) []string {
	var tokens []string

	// Split on whitespace first
	words := strings.Fields(text)

	for _, word := range words {
		// Tokenize each word using WordPiece
		wordTokens := t.wordPieceTokenize(word)
		tokens = append(tokens, wordTokens...)
	}

	return tokens
}

// wordPieceTokenize applies WordPiece algorithm to a single word.
func (t *BERTTokenizer) wordPieceTokenize(word string) []string {
	if len(word) == 0 {
		return nil
	}

	// Handle punctuation by splitting
	tokens := t.splitOnPunctuation(word)

	var result []string
	for _, token := range tokens {
		if len(token) == 0 {
			continue
		}

		// Check if whole token is in vocab
		if _, ok := t.vocab[token]; ok {
			result = append(result, token)
			continue
		}

		// Apply WordPiece algorithm
		subTokens := t.applyWordPiece(token)
		result = append(result, subTokens...)
	}

	return result
}

// splitOnPunctuation splits a word around punctuation marks.
func (t *BERTTokenizer) splitOnPunctuation(word string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range word {
		if t.isPunctuation(r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// isPunctuation checks if a rune is a punctuation character.
func (t *BERTTokenizer) isPunctuation(r rune) bool {
	// Check Unicode punctuation categories
	if unicode.IsPunct(r) {
		return true
	}
	// Additional characters considered punctuation
	code := int(r)
	if (code >= 33 && code <= 47) || // !"#$%&'()*+,-./
		(code >= 58 && code <= 64) || // :;<=>?@
		(code >= 91 && code <= 96) || // [\]^_`
		(code >= 123 && code <= 126) { // {|}~
		return true
	}
	return false
}

// applyWordPiece applies the WordPiece algorithm to break unknown words into subwords.
func (t *BERTTokenizer) applyWordPiece(word string) []string {
	var tokens []string

	runes := []rune(word)
	start := 0

	for start < len(runes) {
		end := len(runes)
		found := false

		for end > start {
			substr := string(runes[start:end])
			if start > 0 {
				substr = SubwordPrefix + substr
			}

			if _, ok := t.vocab[substr]; ok {
				tokens = append(tokens, substr)
				found = true
				break
			}
			end--
		}

		if !found {
			// Character not in vocab, use [UNK]
			if start == 0 {
				tokens = append(tokens, TokenUNK)
			} else {
				tokens = append(tokens, SubwordPrefix+TokenUNK)
			}
			start++
		} else {
			start = end
		}
	}

	return tokens
}

// tokenToID converts a token string to its vocabulary ID.
func (t *BERTTokenizer) tokenToID(token string) int {
	if id, ok := t.vocab[token]; ok {
		return id
	}
	return t.unkID
}

// Decode converts token IDs back to text (for debugging).
func (t *BERTTokenizer) Decode(ids []int64) string {
	var tokens []string
	for _, id := range ids {
		if token, ok := t.reverseVocab[int(id)]; ok {
			// Skip special tokens
			if token == TokenCLS || token == TokenSEP || token == TokenPAD {
				continue
			}
			tokens = append(tokens, token)
		}
	}

	// Join tokens, handling subword prefixes
	var result strings.Builder
	for i, token := range tokens {
		if strings.HasPrefix(token, SubwordPrefix) {
			result.WriteString(strings.TrimPrefix(token, SubwordPrefix))
		} else {
			if i > 0 {
				result.WriteString(" ")
			}
			result.WriteString(token)
		}
	}

	return result.String()
}

// VocabSize returns the vocabulary size.
func (t *BERTTokenizer) VocabSize() int {
	return len(t.vocab)
}

// MaxLength returns the maximum sequence length.
func (t *BERTTokenizer) MaxLength() int {
	return t.maxLen
}
