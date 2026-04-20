package queue

import (
	"context"

	"github.com/andro/rag/internal/domain"
)

type MemoryQueue struct {
	ch chan domain.IngestionJob
}

func NewMemoryQueue(buffer int) *MemoryQueue {
	if buffer <= 0 {
		buffer = 128
	}
	return &MemoryQueue{ch: make(chan domain.IngestionJob, buffer)}
}

func (q *MemoryQueue) Enqueue(ctx context.Context, job domain.IngestionJob) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.ch <- job:
		return nil
	}
}

func (q *MemoryQueue) Consume(ctx context.Context, handler func(context.Context, domain.IngestionJob) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-q.ch:
			_ = handler(ctx, job)
		}
	}
}
