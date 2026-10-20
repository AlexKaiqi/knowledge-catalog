package opensearchimpl

import "unicode/utf8"

// The configured embedding model rejects a request above its context window.
// These caps stay under that window for the deployed model (about 32k tokens)
// without depending on a tokenizer: one long document is cut, and a call packs
// several shorter ones only while their combined length stays inside the budget.
const (
	embeddingTextRunes    = 6000
	embeddingRequestRunes = 12000
)

func embeddingRequests(texts []string) [][]string {
	batches := [][]string{}
	batch := []string{}
	used := 0
	for _, text := range texts {
		text = truncateRunes(text, embeddingTextRunes)
		n := utf8.RuneCountInString(text)
		if len(batch) > 0 && used+n > embeddingRequestRunes {
			batches = append(batches, batch)
			batch = nil
			used = 0
		}
		batch = append(batch, text)
		used += n
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}

func truncateRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	count := 0
	for pos := range text {
		if count == limit {
			return text[:pos]
		}
		count++
	}
	return text
}
