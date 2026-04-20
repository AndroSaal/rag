package embedding

import (
	"context"
	"hash/fnv"

	"github.com/andro/rag/internal/domain"
)

type LocalProvider struct{}

func NewLocalProvider() *LocalProvider { return &LocalProvider{} }

func (l *LocalProvider) EmbedTexts(_ context.Context, _ domain.EmbeddingModelInfo, texts []string) ([][]float64, []int, error) {
	out := make([][]float64, len(texts))
	for i, t := range texts {
		out[i] = hashEmbedding(t, 16)
	}
	return out, nil, nil
}

func hashEmbedding(s string, dim int) []float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	base := h.Sum64()
	v := make([]float64, dim)
	for i := 0; i < dim; i++ {
		v[i] = float64((base>>uint(i%8))&255) / 255.0
	}
	return v
}
