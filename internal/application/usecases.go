package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/andro/rag/internal/domain"
)

type IngestDocumentUseCase struct {
	Chunker    domain.Chunker
	Embeddings domain.EmbeddingProvider
	Vectors    domain.VectorStore
	Docs       domain.DocumentRepository
	Model      domain.EmbeddingModelInfo
	Logger     *slog.Logger
}

func (uc IngestDocumentUseCase) Execute(ctx context.Context, cmd IngestDocumentCommand) (string, error) {
	docID := cmd.DocumentID
	if docID == "" {
		docID = uuid.NewString()
	}
	now := time.Now().UTC()
	doc := domain.Document{
		ID: docID, TenantID: cmd.TenantID, Source: cmd.Source, Language: cmd.Language, Tags: cmd.Tags, Content: cmd.Content, CreatedAt: now,
	}
	if err := uc.Docs.SaveDocument(ctx, doc); err != nil {
		return "", err
	}
	chunks, err := uc.Chunker.Split(doc)
	if err != nil {
		return "", err
	}
	if len(chunks) == 0 {
		return docID, nil
	}
	if err := uc.Docs.SaveChunks(ctx, chunks); err != nil {
		return "", err
	}
	texts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		texts = append(texts, c.Text)
	}
	vectors, partialFailed, err := uc.Embeddings.EmbedTexts(ctx, uc.Model, texts)
	if err != nil {
		return "", err
	}
	failed := map[int]struct{}{}
	for _, i := range partialFailed {
		failed[i] = struct{}{}
	}
	records := make([]domain.VectorRecord, 0, len(chunks))
	for i, c := range chunks {
		if _, skip := failed[i]; skip {
			if uc.Logger != nil {
				uc.Logger.Warn("embedding partial failure", "chunk_id", c.ID)
			}
			continue
		}
		records = append(records, domain.VectorRecord{
			ID:     c.ID,
			Vector: vectors[i],
			Payload: map[string]any{
				"tenant_id":   c.TenantID,
				"document_id": c.DocumentID,
				"chunk_id":    c.ID,
				"source":      c.Source,
				"language":    c.Language,
				"tags":        c.Tags,
				"created_at":  c.CreatedAt.Unix(),
				"text":        c.Text,
			},
		})
	}
	return docID, uc.Vectors.Upsert(ctx, cmd.TenantID, records)
}

type EnqueueIngestionUseCase struct {
	Queue domain.IngestionQueue
}

func (uc EnqueueIngestionUseCase) Execute(ctx context.Context, cmd IngestDocumentCommand) (jobID string, documentID string, err error) {
	documentID = cmd.DocumentID
	if documentID == "" {
		documentID = uuid.NewString()
	}
	jobID = uuid.NewString()
	err = uc.Queue.Enqueue(ctx, domain.IngestionJob{
		JobID: jobID,
		Document: domain.Document{
			ID: documentID, TenantID: cmd.TenantID, Source: cmd.Source, Language: cmd.Language, Tags: cmd.Tags, Content: cmd.Content, CreatedAt: time.Now().UTC(),
		},
	})
	return jobID, documentID, err
}

type RetrieveUseCase struct {
	Embeddings domain.EmbeddingProvider
	Vectors    domain.VectorStore
	Docs       domain.DocumentRepository
	Reranker   domain.Reranker
	Model      domain.EmbeddingModelInfo
}

func (uc RetrieveUseCase) Execute(ctx context.Context, q RetrieveQuery) (RetrieveResult, error) {
	if q.TopK <= 0 {
		q.TopK = 5
	}
	vectors, _, err := uc.Embeddings.EmbedTexts(ctx, uc.Model, []string{q.Query})
	if err != nil {
		return RetrieveResult{}, err
	}
	filter := map[string]any{"tenant_id": q.TenantID}
	if q.Filter.Source != "" {
		filter["source"] = q.Filter.Source
	}
	if len(q.Filter.Tags) > 0 {
		filter["tags"] = q.Filter.Tags
	}
	results, _, err := uc.Vectors.Search(ctx, q.TenantID, vectors[0], q.TopK, filter, "")
	if err != nil {
		return RetrieveResult{}, err
	}
	ids := make([]string, 0, len(results))
	scores := map[string]float64{}
	for _, r := range results {
		ids = append(ids, r.ID)
		scores[r.ID] = r.Score
	}
	chunks, err := uc.Docs.GetChunksByIDs(ctx, q.TenantID, ids)
	if err != nil {
		return RetrieveResult{}, err
	}
	retrieved := make([]domain.RetrievedChunk, 0, len(chunks))
	for _, c := range chunks {
		retrieved = append(retrieved, domain.RetrievedChunk{Chunk: c, Score: scores[c.ID]})
	}
	if uc.Reranker != nil {
		retrieved, err = uc.Reranker.Rerank(ctx, q.Query, retrieved)
		if err != nil {
			return RetrieveResult{}, err
		}
	}
	sort.Slice(retrieved, func(i, j int) bool {
		if retrieved[i].RerankScore == retrieved[j].RerankScore {
			return retrieved[i].Score > retrieved[j].Score
		}
		return retrieved[i].RerankScore > retrieved[j].RerankScore
	})
	return RetrieveResult{QueryID: uuid.NewString(), Chunks: retrieved}, nil
}

type ChatUseCase struct {
	Retrieve RetrieveUseCase
	LLM      domain.LLMGateway
}

func (uc ChatUseCase) Execute(ctx context.Context, cmd ChatCommand) (<-chan domain.LLMStreamChunk, error) {
	res, err := uc.Retrieve.Execute(ctx, RetrieveQuery{
		TenantID: cmd.TenantID,
		Query:    cmd.Query,
		TopK:     cmd.TopK,
		Filter: domain.RetrievalFilter{
			TenantID: cmd.TenantID,
			Source:   cmd.Source,
			Tags:     cmd.Tags,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(res.Chunks) == 0 || res.Chunks[0].Score < 0.2 {
		ch := make(chan domain.LLMStreamChunk, 1)
		ch <- domain.LLMStreamChunk{Text: "Недостаточно контекста для уверенного ответа.", Done: true}
		close(ch)
		return ch, nil
	}
	var b strings.Builder
	tokenBudget := cmd.MaxContextTokens
	if tokenBudget <= 0 {
		tokenBudget = 1500
	}
	used := 0
	for _, c := range res.Chunks {
		tokens := estimateTokens(c.Chunk.Text)
		if used+tokens > tokenBudget {
			break
		}
		used += tokens
		b.WriteString(c.Chunk.Text)
		b.WriteString("\n---\n")
	}
	if b.Len() == 0 {
		return nil, errors.New("context budget exhausted")
	}
	system := strings.Join([]string{
		"Ты ассистент RAG. Отвечай ТОЛЬКО на русском языке.",
		"Используй ТОЛЬКО факты из предоставленного контекста. Не используй внешние знания, не придумывай детали.",
		"Каждый факт в ответе должен быть привязан к источнику в контексте: если в контексте есть тег вида [msg_id=...], укажи его для каждого факта в формате msg_id=... (можно в скобках).",
		"Если в контексте нет msg_id (или нельзя однозначно сопоставить факт с msg_id из контекста), не выдумывай msg_id и не утверждай факты без привязки: напиши ровно: \"В предоставленном контексте нет данных для ответа.\"",
		"Если в контексте нет данных для ответа, напиши ровно: \"В предоставленном контексте нет данных для ответа.\"",
		"Не пиши шаблоны, SEO-гайды, инструкции, таблицы про другие города, и не переключайся на английский.",
	}, " ")
	msgs := append([]domain.ChatMessage{{Role: "system", Content: system}}, cmd.History...)
	msgs = append(msgs, domain.ChatMessage{Role: "user", Content: fmt.Sprintf("Контекст:\n%s\n\nВопрос:\n%s", b.String(), cmd.Query)})
	return uc.LLM.StreamCompletion(ctx, domain.LLMRequest{TenantID: cmd.TenantID, Messages: msgs})
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return len([]rune(s))/4 + 1
}

type FeedbackUseCase struct {
	Repo domain.FeedbackRepository
}

func (uc FeedbackUseCase) Execute(ctx context.Context, cmd FeedbackCommand) error {
	if cmd.Score < 1 || cmd.Score > 5 {
		return errors.New("score must be in range [1..5]")
	}
	return uc.Repo.SaveFeedback(ctx, domain.Feedback{
		ID: uuid.NewString(), TenantID: cmd.TenantID, QueryID: cmd.QueryID, Score: cmd.Score, Comment: cmd.Comment, CreatedAt: time.Now().UTC(),
	})
}
