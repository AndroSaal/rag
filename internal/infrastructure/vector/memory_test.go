package vector

import (
	"context"
	"testing"

	"github.com/andro/rag/internal/domain"
)

func TestMemoryStoreSearchAndDelete(t *testing.T) {
	s := NewMemoryStore()
	err := s.Upsert(context.Background(), "t1", []domain.VectorRecord{
		{ID: "a", Vector: []float64{1, 0}, Payload: map[string]any{"document_id": "d1"}},
		{ID: "b", Vector: []float64{0, 1}, Payload: map[string]any{"document_id": "d2"}},
	})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	res, _, err := s.Search(context.Background(), "t1", []float64{1, 0}, 1, nil, "")
	if err != nil || len(res) != 1 || res[0].ID != "a" {
		t.Fatalf("unexpected search result: %+v err=%v", res, err)
	}
	if err := s.DeleteByDocument(context.Background(), "t1", "d1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	res, _, _ = s.Search(context.Background(), "t1", []float64{1, 0}, 10, nil, "")
	for _, r := range res {
		if r.ID == "a" {
			t.Fatalf("expected deleted record not to appear")
		}
	}
}
