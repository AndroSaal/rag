package application

import (
	"context"
	"testing"

	"github.com/andro/rag/internal/domain"
	"github.com/andro/rag/internal/infrastructure/embedding"
	"github.com/andro/rag/internal/infrastructure/repository"
	"github.com/andro/rag/internal/infrastructure/reranker"
	"github.com/andro/rag/internal/infrastructure/vector"
)

func TestProviderSwappingAndRetrieval(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewMemoryRepository()
	store := vector.NewMemoryStore()
	embed := embedding.NewLocalProvider()
	model := domain.EmbeddingModelInfo{Provider: "local", Model: "local", Version: "test"}

	ingest := IngestDocumentUseCase{
		Chunker:    OverlapChunker{ChunkSize: 20, Overlap: 4},
		Embeddings: embed, Vectors: store, Docs: repo, Model: model,
	}
	_, err := ingest.Execute(ctx, IngestDocumentCommand{
		TenantID: "tenant-a",
		Content:  "Go language supports interfaces and dependency inversion for clean architecture",
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	retrieve := RetrieveUseCase{
		Embeddings: embed, Vectors: store, Docs: repo, Reranker: reranker.NewSimple(), Model: model,
	}
	res, err := retrieve.Execute(ctx, RetrieveQuery{
		TenantID: "tenant-a", Query: "dependency inversion in go", TopK: 3, Filter: domain.RetrievalFilter{TenantID: "tenant-a"},
	})
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}
	if len(res.Chunks) == 0 {
		t.Fatalf("expected retrieval results")
	}
}

type brokenFeedbackRepo struct{}

func (brokenFeedbackRepo) SaveFeedback(context.Context, domain.Feedback) error {
	return context.DeadlineExceeded
}

func TestFeedbackErrorPropagation(t *testing.T) {
	uc := FeedbackUseCase{Repo: brokenFeedbackRepo{}}
	err := uc.Execute(context.Background(), FeedbackCommand{TenantID: "t", QueryID: "q", Score: 4, Comment: "ok"})
	if err == nil {
		t.Fatalf("expected error")
	}
}
