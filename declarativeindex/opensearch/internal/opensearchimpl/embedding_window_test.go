package opensearchimpl

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEmbeddingRequestsPackByRuneBudgetAndTruncate(t *testing.T) {
	long := strings.Repeat("文", embeddingTextRunes+50)
	batches := embeddingRequests([]string{long, "短", strings.Repeat("字", embeddingRequestRunes)})
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want 2", len(batches))
	}
	if got := utf8.RuneCountInString(batches[0][0]); got != embeddingTextRunes {
		t.Fatalf("truncated runes = %d, want %d", got, embeddingTextRunes)
	}
	if batches[0][1] != "短" {
		t.Fatalf("short text = %q", batches[0][1])
	}
	if got := utf8.RuneCountInString(batches[1][0]); got != embeddingTextRunes {
		t.Fatalf("second batch runes = %d, want %d", got, embeddingTextRunes)
	}
	for _, batch := range batches {
		total := 0
		for _, text := range batch {
			total += utf8.RuneCountInString(text)
		}
		if total > embeddingRequestRunes {
			t.Fatalf("batch runes = %d, over %d", total, embeddingRequestRunes)
		}
	}
}
