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

type GigaProvider struct {
	url    string
	apiKey string
	client *http.Client
}

func NewGigaProvider(url, key string, timeout time.Duration) *GigaProvider {
	return &GigaProvider{url: url, apiKey: key, client: &http.Client{Timeout: timeout}}
}

func (g *GigaProvider) EmbedTexts(ctx context.Context, model domain.EmbeddingModelInfo, texts []string) ([][]float64, []int, error) {
	reqBody := map[string]any{"model": model.Model, "version": model.Version, "input": texts}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
			Error     string    `json:"error,omitempty"`
		} `json:"data"`
	}
	err := withRetry(ctx, 3, func() error { return g.call(ctx, reqBody, &out) })
	if err != nil {
		return nil, nil, err
	}
	vectors := make([][]float64, len(texts))
	partial := []int{}
	for _, d := range out.Data {
		if d.Error != "" {
			partial = append(partial, d.Index)
			continue
		}
		vectors[d.Index] = d.Embedding
	}
	for i := range vectors {
		if vectors[i] == nil {
			partial = append(partial, i)
		}
	}
	return vectors, partial, nil
}

func (g *GigaProvider) call(ctx context.Context, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("embedding status=%d body=%s", resp.StatusCode, string(raw))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
