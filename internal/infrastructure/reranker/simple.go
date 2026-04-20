package reranker

import (
	"context"
	"strings"

	"github.com/andro/rag/internal/domain"
)

type SimpleReranker struct{}

func NewSimple() *SimpleReranker { return &SimpleReranker{} }

func (s *SimpleReranker) Rerank(_ context.Context, query string, chunks []domain.RetrievedChunk) ([]domain.RetrievedChunk, error) {
	qwords := words(query)
	for i := range chunks {
		cwords := words(chunks[i].Chunk.Text)
		match := 0
		for w := range qwords {
			if _, ok := cwords[w]; ok {
				match++
			}
		}
		chunks[i].RerankScore = chunks[i].Score + float64(match)*0.01
	}
	return chunks, nil
}

func words(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		out[w] = struct{}{}
	}
	return out
}
