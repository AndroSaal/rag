**[Switch to Russian](README.ru.md)**

# Generic RAG Platform (Go)

Production-ready template for a reusable RAG backend with clean architecture and swappable providers.

## Architecture

- `internal/domain`: entities and interfaces (`EmbeddingProvider`, `VectorStore`, `LLMGateway`, repositories).
- `internal/application`: use cases and orchestration (`Ingest`, `Retrieve`, `Chat`, `Feedback`).
- `internal/infrastructure`: adapters (Qdrant, GigaEmbeddings, HTTP LLM gateway, repositories, config).
- `internal/interfaces`: transport layer (`REST` handlers + SSE streaming).
- `cmd/server`: dependency injection and bootstrap.

The application layer depends only on domain interfaces. Replacing providers affects only infrastructure adapters and `main` wiring.

## Features

- Async document ingestion with queue/worker (`POST /documents`)
- Chunking with configurable size/overlap
- Embedding generation with batching contract
- Vector upsert/search/delete abstraction
- Metadata-aware semantic retrieval (`POST /retrieve`)
- Optional reranking component (independent adapter)
- Context budget management for prompts
- Guardrail for low-confidence retrieval
- Streaming chat response via SSE (`POST /chat`)
- Feedback persistence (`POST /feedback`)
- Health/readiness probes
- Metrics endpoint (`/metrics`)
- OpenAPI spec (`docs/openapi.yaml`)

## End-to-end RAG pipeline

1. Client sends document to `/documents`.
2. API enqueues ingestion job and immediately returns `job_id` + `document_id`.
3. Background worker executes chunking, embedding, metadata save and vector upsert.
4. `/retrieve` performs semantic search with filters.
5. Chat use case builds bounded context from retrieved chunks.
6. LLM gateway streams answer tokens to client.
7. `/feedback` stores quality signals for evaluation loops.

## Run locally

```bash
cp config.example.env .env
go mod tidy
go test ./...
go run ./cmd/server
```

## Run with Docker Compose

```bash
make up
make ps
make smoke-test
```

### OpenRouter mandatory config

For predictable behavior with OpenRouter, set all of these:

```bash
LLM_PROVIDER=openrouter
LLM_URL=https://openrouter.ai/api/v1/chat/completions
LLM_API_KEY=sk-or-v1-...
OPENROUTER_HTTP_REFERER=https://your-domain.example
OPENROUTER_APP_NAME=your-rag-app

EMBEDDING_PROVIDER=openrouter
EMBEDDING_URL=https://openrouter.ai/api/v1/embeddings
EMBEDDING_MODEL=intfloat/multilingual-e5-large
# optional, falls back to LLM_API_KEY when empty
EMBEDDING_API_KEY=
```

Server validates this on startup and fails fast if required OpenRouter values are missing.

What these commands do:

- `make up`: starts `app + qdrant + postgres + redis`.
- `make ensure-collection`: creates Qdrant collection if missing.
- `make recreate-collection`: recreates Qdrant collection from scratch.
- `make smoke-test`: runs end-to-end check (`health -> ingest -> retrieve -> chat`).

Useful operations:

```bash
make logs
make restart
make down
```

Default local setup uses in-memory providers if:

```bash
export LLM_PROVIDER=local
export EMBEDDING_PROVIDER=local
export VECTOR_PROVIDER=memory
```

## Provider switching

No use-case changes required:

- Embeddings:
  - `EMBEDDING_PROVIDER=giga` -> `internal/infrastructure/embedding/giga.go`
  - `EMBEDDING_PROVIDER=local` -> `internal/infrastructure/embedding/local.go`
- Vector DB:
  - `VECTOR_PROVIDER=qdrant` -> `internal/infrastructure/vector/qdrant.go`
  - `VECTOR_PROVIDER=memory` -> `internal/infrastructure/vector/memory.go`
- LLM:
  - `LLM_PROVIDER=giga` (HTTP gateway adapter)
  - `LLM_PROVIDER=local` (test/local adapter)

To add a new provider, implement the corresponding domain interface and register it in `cmd/server/main.go`.

## Embedding dimension and full reindex (required)

`VECTOR_SIZE` must match embedding output size exactly.  
For `intfloat/multilingual-e5-large`, use `VECTOR_SIZE=1024`.

When switching embedding model:

1. Update `.env` (`EMBEDDING_MODEL`, `VECTOR_SIZE`).
2. Recreate Qdrant collection:
   ```bash
   make recreate-collection VECTOR_SIZE=1024
   ```
3. Reindex all documents (ingest again), otherwise semantic search quality will break.

If you ingest Telegram data, re-run `scripts/telegram_ingest.go` on your export after collection recreation.

## API endpoints

- `POST /documents`
- `POST /retrieve`
- `POST /chat` (SSE stream)
- `POST /feedback`
- `GET /health`
- `GET /ready`

## How you "train" this RAG (enrich vector store)

In RAG, "training" usually means **indexing your knowledge base**, not model fine-tuning.

1. Prepare your source documents (docs, FAQs, wiki pages, manuals, tickets).
2. Send each document to `POST /documents` with:
   - `tenant_id` (project/workspace isolation),
   - `source` (where document came from),
   - `language`,
   - `tags`,
   - `content`.
3. The service asynchronously:
   - chunks content,
   - builds embeddings,
   - upserts vectors + metadata to vector DB.
4. After ingestion, your assistant can answer using `/retrieve` + `/chat`.

Minimal ingestion example:

```bash
curl -X POST "http://localhost:8080/documents" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id":"acme-prod",
    "source":"confluence",
    "language":"ru",
    "tags":["billing","support"],
    "content":"Текст вашей базы знаний..."
  }'
```

## How to connect from your production service

Typical integration pattern:

1. Your backend stores tenant/user/session context.
2. On user question, backend calls `POST /chat` with:
   - `tenant_id`,
   - user `query`,
   - optional `source` and `tags` for retrieval filtering,
   - optional `history`,
   - optional `max_context_tokens`.
3. Backend proxies SSE stream to frontend (web/mobile) as live assistant response.
4. After response, backend sends quality signal to `POST /feedback`.

Recommended production flow:

- **Ingestion pipeline**: trigger `/documents` from ETL/CDC/jobs when content changes.
- **Tenant isolation**: map each customer/project to own `tenant_id`.
- **Observability**: collect `/metrics`, logs, traces.
- **Provider swap**: switch providers through env variables without changing use cases.

## Production checklist

- Set non-local providers:
  - `EMBEDDING_PROVIDER=openrouter` (or your embedding provider)
  - `VECTOR_PROVIDER=qdrant`
  - `LLM_PROVIDER=openrouter` (or your LLM provider)
- Configure real credentials (`*_API_KEY`) and private network endpoints.
- Run DB migrations for PostgreSQL.
- Configure TLS/Ingress/API Gateway in front of this service.
- Add authN/authZ middleware for API access.
- Add retries/circuit-breaker limits in your caller service.
- Do not commit `.env`; rotate keys immediately if a key was exposed.

## CLI channel filtering (`source` / `tags`)

`scripts/chat_cli.sh` can force retrieval to a channel/source:

```bash
scripts/chat_cli.sh -t telegram-moskva -s "telegram:@my_channel" -g "news,weather" -q "Что писали про снег?"
```

Important: pass the same `source` value you used at ingestion time (for Telegram ingest: `-source` in `scripts/telegram_ingest.go`, e.g. `telegram:@my_channel`).

## Notes on production hardening

- Repository includes Postgres adapter (`internal/infrastructure/repository/postgres.go`) and SQL migration in `migrations`.
- Add Redis-based locks/rate-limits as another infrastructure adapter (use cases stay unchanged).
- Add queue workers for heavy ingestion by moving ingest orchestration into async command handlers with the same `IngestDocumentUseCase`.
