package vector

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

type QdrantStore struct {
	baseURL    string
	apiKey     string
	collection string
	client     *http.Client
}

func NewQdrantStore(baseURL, apiKey, collection string, timeout time.Duration) *QdrantStore {
	return &QdrantStore{
		baseURL: baseURL, apiKey: apiKey, collection: collection,
		client: &http.Client{Timeout: timeout},
	}
}

func (q *QdrantStore) Upsert(ctx context.Context, namespace string, records []domain.VectorRecord) error {
	points := make([]map[string]any, 0, len(records))
	for _, r := range records {
		payload := clonePayload(r.Payload)
		payload["namespace"] = namespace
		points = append(points, map[string]any{"id": r.ID, "vector": r.Vector, "payload": payload})
	}
	body := map[string]any{"points": points}
	return q.call(ctx, http.MethodPut, fmt.Sprintf("%s/collections/%s/points", q.baseURL, q.collection), body, nil)
}

func (q *QdrantStore) Search(ctx context.Context, namespace string, vector []float64, topK int, filter map[string]any, _ string) ([]domain.VectorSearchResult, string, error) {
	must := []map[string]any{{"key": "namespace", "match": map[string]any{"value": namespace}}}
	for k, v := range filter {
		must = append(must, map[string]any{"key": k, "match": map[string]any{"value": v}})
	}
	req := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": true,
		"filter":       map[string]any{"must": must},
	}
	var resp struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	err := q.call(ctx, http.MethodPost, fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, q.collection), req, &resp)
	if err != nil {
		return nil, "", err
	}
	out := make([]domain.VectorSearchResult, 0, len(resp.Result))
	for _, r := range resp.Result {
		out = append(out, domain.VectorSearchResult{ID: fmt.Sprint(r.ID), Score: r.Score, Payload: r.Payload})
	}
	return out, "", nil
}

func (q *QdrantStore) DeleteByDocument(ctx context.Context, namespace, documentID string) error {
	body := map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": "namespace", "match": map[string]any{"value": namespace}},
				{"key": "document_id", "match": map[string]any{"value": documentID}},
			},
		},
	}
	return q.call(ctx, http.MethodPost, fmt.Sprintf("%s/collections/%s/points/delete", q.baseURL, q.collection), body, nil)
}

func (q *QdrantStore) call(ctx context.Context, method, url string, reqBody any, out any) error {
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant error: status=%d body=%s", resp.StatusCode, string(raw))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func clonePayload(p map[string]any) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}
