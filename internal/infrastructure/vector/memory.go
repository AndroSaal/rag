package vector

import (
	"context"
	"math"
	"sort"
	"sync"

	"github.com/andro/rag/internal/domain"
)

type MemoryStore struct {
	mu   sync.RWMutex
	data map[string][]domain.VectorRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[string][]domain.VectorRecord{}}
}

func (m *MemoryStore) Upsert(_ context.Context, namespace string, records []domain.VectorRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[namespace] = append(m.data[namespace], records...)
	return nil
}

func (m *MemoryStore) Search(_ context.Context, namespace string, vector []float64, topK int, _ map[string]any, _ string) ([]domain.VectorSearchResult, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]domain.VectorSearchResult, 0, len(m.data[namespace]))
	for _, r := range m.data[namespace] {
		res = append(res, domain.VectorSearchResult{ID: r.ID, Score: cosine(vector, r.Vector), Payload: r.Payload})
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Score > res[j].Score })
	if topK > 0 && topK < len(res) {
		res = res[:topK]
	}
	return res, "", nil
}

func (m *MemoryStore) DeleteByDocument(_ context.Context, namespace, documentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := m.data[namespace][:0]
	for _, r := range m.data[namespace] {
		if r.Payload["document_id"] == documentID {
			continue
		}
		filtered = append(filtered, r)
	}
	m.data[namespace] = filtered
	return nil
}

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := 0; i < len(a) && i < len(b); i++ {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
