package main

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/andro/rag/internal/application"
	"github.com/andro/rag/internal/domain"
	"github.com/andro/rag/internal/infrastructure/config"
	"github.com/andro/rag/internal/infrastructure/embedding"
	"github.com/andro/rag/internal/infrastructure/llm"
	"github.com/andro/rag/internal/infrastructure/observability"
	"github.com/andro/rag/internal/infrastructure/queue"
	"github.com/andro/rag/internal/infrastructure/repository"
	"github.com/andro/rag/internal/infrastructure/reranker"
	"github.com/andro/rag/internal/infrastructure/vector"
	"github.com/andro/rag/internal/interfaces/httpapi"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	shutdownTracer, err := observability.InitTracer("generic-rag")
	if err != nil {
		log.Fatal(err)
	}
	defer shutdownTracer(context.Background())

	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	docRepo := repository.NewPostgresRepository(db)
	ingestionQueue := queue.NewMemoryQueue(256)

	var embedProvider domain.EmbeddingProvider
	switch strings.ToLower(strings.TrimSpace(cfg.ProviderEmbedding)) {
	case "local":
		embedProvider = embedding.NewLocalProvider()
	case "openai", "openrouter":
		embedProvider = embedding.NewOpenAICompatibleEmbeddings(
			cfg.EmbeddingURL,
			cfg.EmbeddingKey,
			cfg.EmbeddingModel,
			cfg.OpenRouterHTTPReferer,
			cfg.OpenRouterAppName,
			cfg.RequestTimeout,
		)
	default:
		embedProvider = embedding.NewGigaProvider(cfg.GigaEmbeddingsURL, cfg.GigaEmbeddingsKey, cfg.RequestTimeout)
	}

	var vectorStore domain.VectorStore
	switch cfg.ProviderVectorDB {
	case "memory":
		vectorStore = vector.NewMemoryStore()
	default:
		vectorStore = vector.NewQdrantStore(cfg.QdrantURL, cfg.QdrantKey, cfg.QdrantCollection, cfg.RequestTimeout)
	}

	var llmGateway domain.LLMGateway
	switch cfg.ProviderLLM {
	case "local":
		llmGateway = llm.NewLocalGateway()
	default:
		llmGateway = llm.NewHTTPGateway(
			cfg.LLMURL,
			cfg.LLMKey,
			cfg.LLMModel,
			cfg.LLMBackoffModels,
			cfg.OpenRouterHTTPReferer,
			cfg.OpenRouterAppName,
			cfg.RequestTimeout,
		)
	}

	model := domain.EmbeddingModelInfo{
		Provider: cfg.ProviderEmbedding,
		Model:    cfg.GigaEmbeddingsModel,
		Version:  cfg.GigaEmbeddingsVersion,
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ProviderEmbedding)) {
	case "openai", "openrouter":
		model = domain.EmbeddingModelInfo{
			Provider: cfg.ProviderEmbedding,
			Model:    cfg.EmbeddingModel,
			Version:  "v1",
		}
	case "local":
		model = domain.EmbeddingModelInfo{Provider: "local", Model: "local-hash", Version: "v1"}
	}
	retrieveUC := application.RetrieveUseCase{
		Embeddings: embedProvider,
		Vectors:    vectorStore,
		Docs:       docRepo,
		Reranker:   reranker.NewSimple(),
		Model:      model,
	}
	handlers := httpapi.Handlers{
		Ingest: application.IngestDocumentUseCase{
			Chunker:    application.OverlapChunker{ChunkSize: cfg.ChunkSize, Overlap: cfg.ChunkOverlap},
			Embeddings: embedProvider,
			Vectors:    vectorStore,
			Docs:       docRepo,
			Model:      model,
			Logger:     logger,
		},
		Enqueue:  application.EnqueueIngestionUseCase{Queue: ingestionQueue},
		Retrieve: retrieveUC,
		Chat:     application.ChatUseCase{Retrieve: retrieveUC, LLM: llmGateway},
		Feedback: application.FeedbackUseCase{Repo: docRepo},
		TopK:     cfg.DefaultTopK,
		Timeout:  cfg.RequestTimeout,
	}

	go ingestionQueue.Consume(context.Background(), func(ctx context.Context, job domain.IngestionJob) error {
		_, err := handlers.Ingest.Execute(ctx, application.IngestDocumentCommand{
			DocumentID: job.Document.ID,
			TenantID:   job.Document.TenantID,
			Source:     job.Document.Source,
			Language:   job.Document.Language,
			Tags:       job.Document.Tags,
			Content:    job.Document.Content,
		})
		if err != nil {
			logger.Error("ingestion job failed", "job_id", job.JobID, "document_id", job.Document.ID, "error", err)
			return err
		}
		logger.Info("ingestion job completed", "job_id", job.JobID, "document_id", job.Document.ID)
		return nil
	})

	logger.Info("starting rag server", "addr", cfg.HTTPAddr)
	wrapped := otelhttp.NewHandler(handlers.Router(), "rag-http")
	if err := http.ListenAndServe(cfg.HTTPAddr, wrapped); err != nil {
		log.Fatal(err)
	}
}
