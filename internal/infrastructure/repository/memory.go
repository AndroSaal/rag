package repository

import (
	"context"
	"sync"

	"github.com/andro/rag/internal/domain"
)

type MemoryRepository struct {
	mu        sync.RWMutex
	docs      map[string]domain.Document
	chunks    map[string]domain.Chunk
	feedbacks map[string]domain.Feedback
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		docs:      map[string]domain.Document{},
		chunks:    map[string]domain.Chunk{},
		feedbacks: map[string]domain.Feedback{},
	}
}

func (r *MemoryRepository) SaveDocument(_ context.Context, doc domain.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs[doc.ID] = doc
	return nil
}

func (r *MemoryRepository) SaveChunks(_ context.Context, chunks []domain.Chunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range chunks {
		r.chunks[c.ID] = c
	}
	return nil
}

func (r *MemoryRepository) GetChunksByIDs(_ context.Context, tenantID string, ids []string) ([]domain.Chunk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Chunk, 0, len(ids))
	for _, id := range ids {
		c, ok := r.chunks[id]
		if !ok || c.TenantID != tenantID {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *MemoryRepository) SaveFeedback(_ context.Context, feedback domain.Feedback) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feedbacks[feedback.ID] = feedback
	return nil
}
