package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/andro/rag/internal/application"
	"github.com/andro/rag/internal/domain"
)

type Handlers struct {
	Ingest   application.IngestDocumentUseCase
	Enqueue  application.EnqueueIngestionUseCase
	Retrieve application.RetrieveUseCase
	Chat     application.ChatUseCase
	Feedback application.FeedbackUseCase
	TopK     int
	Timeout  time.Duration
}

func (h Handlers) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	})
	r.Post("/documents", h.ingest)
	r.Post("/retrieve", h.retrieve)
	r.Post("/chat", h.chat)
	r.Post("/feedback", h.feedback)
	r.Handle("/metrics", promhttp.Handler())
	return r
}

func (h Handlers) ingest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID string   `json:"tenant_id"`
		Source   string   `json:"source"`
		Language string   `json:"language"`
		Tags     []string `json:"tags"`
		Content  string   `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.TenantID == "" || req.Content == "" {
		writeErr(w, http.StatusBadRequest, "validation_error", "tenant_id and content are required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()
	jobID, id, err := h.Enqueue.Execute(ctx, application.IngestDocumentCommand{
		TenantID: req.TenantID, Source: req.Source, Language: req.Language, Tags: req.Tags, Content: req.Content,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ingest_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "document_id": id})
}

func (h Handlers) retrieve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID string   `json:"tenant_id"`
		Query    string   `json:"query"`
		TopK     int      `json:"top_k"`
		Source   string   `json:"source"`
		Tags     []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.TenantID == "" || req.Query == "" {
		writeErr(w, http.StatusBadRequest, "validation_error", "tenant_id and query are required")
		return
	}
	if req.TopK <= 0 {
		req.TopK = h.TopK
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()
	out, err := h.Retrieve.Execute(ctx, application.RetrieveQuery{
		TenantID: req.TenantID, Query: req.Query, TopK: req.TopK,
		Filter: domain.RetrievalFilter{TenantID: req.TenantID, Source: req.Source, Tags: req.Tags},
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "retrieval_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h Handlers) chat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID         string               `json:"tenant_id"`
		Query            string               `json:"query"`
		Source           string               `json:"source"`
		Tags             []string             `json:"tags"`
		TopK             int                  `json:"top_k"`
		History          []domain.ChatMessage `json:"history"`
		MaxContextTokens int                  `json:"max_context_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.TenantID == "" || req.Query == "" {
		writeErr(w, http.StatusBadRequest, "validation_error", "tenant_id and query are required")
		return
	}
	if req.TopK <= 0 {
		req.TopK = h.TopK
	}
	stream, err := h.Chat.Execute(r.Context(), application.ChatCommand{
		TenantID:         req.TenantID,
		Query:            req.Query,
		Source:           req.Source,
		Tags:             req.Tags,
		TopK:             req.TopK,
		History:          req.History,
		MaxContextTokens: req.MaxContextTokens,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "chat_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "stream_unsupported", "streaming unsupported")
		return
	}
	for chunk := range stream {
		b, _ := json.Marshal(chunk)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		if chunk.Done {
			return
		}
	}
}

func (h Handlers) feedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID string `json:"tenant_id"`
		QueryID  string `json:"query_id"`
		Score    int    `json:"score"`
		Comment  string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	err := h.Feedback.Execute(r.Context(), application.FeedbackCommand{
		TenantID: req.TenantID, QueryID: req.QueryID, Score: req.Score, Comment: req.Comment,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "feedback_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted"})
}

func writeErr(w http.ResponseWriter, code int, kind, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]any{"code": kind, "message": msg}})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
