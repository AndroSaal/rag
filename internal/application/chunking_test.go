package application

import (
	"strings"
	"testing"

	"github.com/andro/rag/internal/domain"
)

func TestOverlapChunkerSplit(t *testing.T) {
	doc := domain.Document{
		ID: "d1", TenantID: "t1", Content: strings.Repeat("слово ", 30),
	}
	chunks, err := OverlapChunker{ChunkSize: 10, Overlap: 2}.Split(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected several chunks, got %d", len(chunks))
	}
	if chunks[0].TenantID != "t1" || chunks[0].DocumentID != "d1" {
		t.Fatalf("metadata not propagated")
	}
}

func TestOverlapChunkerPreservesTelegramMsgMarkers(t *testing.T) {
	sep := "\n\n---\n\n"
	doc := domain.Document{
		ID:       "d1",
		TenantID: "t1",
		Content: strings.Join([]string{
			"[msg_id=1 date=x from=y]\nhello",
			"[msg_id=2 date=x from=y]\nworld",
			"[msg_id=3 date=x from=y]\nmore",
		}, sep),
	}
	chunks, err := OverlapChunker{ChunkSize: 80, Overlap: 10}.Split(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected chunks")
	}
	for _, ch := range chunks {
		if !strings.Contains(ch.Text, "[msg_id=") {
			t.Fatalf("expected msg_id markers to remain intact, got: %q", ch.Text)
		}
	}
}
