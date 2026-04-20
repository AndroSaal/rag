package domain

import (
	"context"
	"time"
)

type Document struct {
	ID        string
	TenantID  string
	Source    string
	Language  string
	Tags      []string
	Content   string
	CreatedAt time.Time
}

type Chunk struct {
	ID         string
	DocumentID string
	TenantID   string
	Text       string
	Index      int
	Source     string
	Language   string
	Tags       []string
	CreatedAt  time.Time
}

type RetrievalFilter struct {
	TenantID string
	Tags     []string
	Source   string
}

type RetrievedChunk struct {
	Chunk       Chunk
	Score       float64
	RerankScore float64
}

type ChatMessage struct {
	Role    string
	Content string
}

type Feedback struct {
	ID        string
	TenantID  string
	QueryID   string
	Score     int
	Comment   string
	CreatedAt time.Time
}

type LLMUsage struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

type LLMStreamChunk struct {
	Text  string
	Done  bool
	Usage *LLMUsage
}

type EmbeddingModelInfo struct {
	Provider string
	Model    string
	Version  string
}

type Chunker interface {
	Split(doc Document) ([]Chunk, error)
}

type EmbeddingProvider interface {
	EmbedTexts(ctx context.Context, model EmbeddingModelInfo, texts []string) (vectors [][]float64, partialFailed []int, err error)
}

type VectorRecord struct {
	ID      string
	Vector  []float64
	Payload map[string]any
}

type VectorSearchResult struct {
	ID      string
	Score   float64
	Payload map[string]any
}

type VectorStore interface {
	Upsert(ctx context.Context, namespace string, records []VectorRecord) error
	Search(ctx context.Context, namespace string, vector []float64, topK int, filter map[string]any, cursor string) (results []VectorSearchResult, nextCursor string, err error)
	DeleteByDocument(ctx context.Context, namespace, documentID string) error
}

type Reranker interface {
	Rerank(ctx context.Context, query string, chunks []RetrievedChunk) ([]RetrievedChunk, error)
}

type LLMGateway interface {
	StreamCompletion(ctx context.Context, req LLMRequest) (<-chan LLMStreamChunk, error)
}

type LLMRequest struct {
	TenantID string
	Messages []ChatMessage
}

type DocumentRepository interface {
	SaveDocument(ctx context.Context, doc Document) error
	SaveChunks(ctx context.Context, chunks []Chunk) error
	GetChunksByIDs(ctx context.Context, tenantID string, ids []string) ([]Chunk, error)
}

type FeedbackRepository interface {
	SaveFeedback(ctx context.Context, feedback Feedback) error
}

type IngestionJob struct {
	JobID    string
	Document Document
}

type IngestionQueue interface {
	Enqueue(ctx context.Context, job IngestionJob) error
	Consume(ctx context.Context, handler func(context.Context, IngestionJob) error)
}
