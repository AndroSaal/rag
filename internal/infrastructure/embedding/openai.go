package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/andro/rag/internal/domain"
)

// OpenAICompatibleEmbeddings calls an OpenAI-compatible /v1/embeddings endpoint.
// Works with OpenRouter: https://openrouter.ai/api/v1/embeddings
type OpenAICompatibleEmbeddings struct {
	url     string
	apiKey  string
	model   string
	siteURL string
	appName string
	client  *http.Client
}

func NewOpenAICompatibleEmbeddings(url, apiKey, model, siteURL, appName string, timeout time.Duration) *OpenAICompatibleEmbeddings {
	return &OpenAICompatibleEmbeddings{
		url:     url,
		apiKey:  apiKey,
		model:   model,
		siteURL: siteURL,
		appName: appName,
		client:  &http.Client{Timeout: timeout},
	}
}

func (p *OpenAICompatibleEmbeddings) EmbedTexts(ctx context.Context, _ domain.EmbeddingModelInfo, texts []string) ([][]float64, []int, error) {
	if len(texts) == 0 {
		return nil, nil, nil
	}
	body := map[string]any{
		"model": p.model,
		"input": texts,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	if p.siteURL != "" {
		req.Header.Set("HTTP-Referer", p.siteURL)
	}
	if p.appName != "" {
		req.Header.Set("X-Title", p.appName)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("embeddings status=%d body=%s", resp.StatusCode, string(raw))
	}

	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Error     any       `json:"error"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, fmt.Errorf("decode embeddings: %w", err)
	}
	if out.Error != nil {
		return nil, nil, fmt.Errorf("embeddings api error: %v", out.Error)
	}

	vectors := make([][]float64, len(texts))
	failed := make([]int, 0)
	for _, item := range out.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			continue
		}
		if item.Error != nil || len(item.Embedding) == 0 {
			failed = append(failed, item.Index)
			continue
		}
		vectors[item.Index] = item.Embedding
	}

	// If API omitted some indices, treat missing as failed.
	for i := range vectors {
		if vectors[i] == nil {
			failed = append(failed, i)
		}
	}
	return vectors, failed, nil
}
