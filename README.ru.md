**[Switch to English](README.md)**

# Универсальная RAG-платформа (Go)

Production-ready шаблон для переиспользуемого RAG backend-сервиса с чистой архитектурой и заменяемыми провайдерами.

## Архитектура

- `internal/domain`: сущности и интерфейсы (`EmbeddingProvider`, `VectorStore`, `LLMGateway`, репозитории).
- `internal/application`: use case-ы и оркестрация (`Ingest`, `Retrieve`, `Chat`, `Feedback`).
- `internal/infrastructure`: адаптеры (Qdrant, GigaEmbeddings, HTTP LLM gateway, репозитории, конфиг).
- `internal/interfaces`: транспортный слой (`REST` + SSE-streaming).
- `cmd/server`: dependency injection и bootstrap приложения.

Слой `application` зависит только от интерфейсов из `domain`. Замена провайдеров затрагивает только инфраструктуру и wiring в `main`.

## Возможности

- Асинхронная загрузка документов через queue/worker (`POST /documents`)
- Чанкинг с настраиваемыми `chunk_size` и `overlap`
- Генерация эмбеддингов через абстракцию провайдера
- Абстракция векторного хранилища (upsert/search/delete)
- Семантический поиск с metadata-фильтрами (`POST /retrieve`)
- Независимый reranker-компонент
- Контроль контекстного бюджета для LLM
- Guardrail при низкой уверенности retrieval
- Потоковый ответ через SSE (`POST /chat`)
- Сбор feedback (`POST /feedback`)
- Health/readiness endpoints
- Метрики (`/metrics`)
- OpenAPI-описание (`docs/openapi.yaml`)

## End-to-end RAG pipeline

1. Клиент отправляет документ в `/documents`.
2. API ставит ingestion job в очередь и сразу возвращает `job_id` + `document_id`.
3. Background worker выполняет чанкинг, эмбеддинги, сохранение metadata и upsert в векторное хранилище.
4. `/retrieve` делает semantic search с фильтрами.
5. Chat use case собирает ограниченный по токенам контекст.
6. LLM gateway отдает ответ стримом.
7. `/feedback` сохраняет оценку качества для последующей аналитики.

## Локальный запуск

```bash
cp config.example.env .env
go mod tidy
go test ./...
go run ./cmd/server
```

## Запуск через Docker Compose

```bash
make up
make ps
make smoke-test
```

### Обязательная конфигурация OpenRouter

Для предсказуемой работы с OpenRouter задайте все поля:

```bash
LLM_PROVIDER=openrouter
LLM_URL=https://openrouter.ai/api/v1/chat/completions
LLM_API_KEY=sk-or-v1-...
OPENROUTER_HTTP_REFERER=https://your-domain.example
OPENROUTER_APP_NAME=your-rag-app

EMBEDDING_PROVIDER=openrouter
EMBEDDING_URL=https://openrouter.ai/api/v1/embeddings
EMBEDDING_MODEL=intfloat/multilingual-e5-large
# опционально, если пусто — берется LLM_API_KEY
EMBEDDING_API_KEY=
```

Сервер валидирует эти поля при старте и завершится с ошибкой, если для OpenRouter не хватает обязательных значений.

Что делают команды:

- `make up`: поднимает `app + qdrant + postgres + redis`.
- `make ensure-collection`: создает коллекцию Qdrant, если ее нет.
- `make recreate-collection`: пересоздает коллекцию Qdrant с нуля.
- `make smoke-test`: прогоняет проверку `health -> ingest -> retrieve -> chat`.

Полезные команды:

```bash
make logs
make restart
make down
```

Для полностью локального режима (без внешних LLM/embedding):

```bash
export LLM_PROVIDER=local
export EMBEDDING_PROVIDER=local
export VECTOR_PROVIDER=memory
```

## Переключение провайдеров

Use case-ы менять не нужно.

- Embeddings:
  - `EMBEDDING_PROVIDER=giga` -> `internal/infrastructure/embedding/giga.go`
  - `EMBEDDING_PROVIDER=local` -> `internal/infrastructure/embedding/local.go`
- Vector DB:
  - `VECTOR_PROVIDER=qdrant` -> `internal/infrastructure/vector/qdrant.go`
  - `VECTOR_PROVIDER=memory` -> `internal/infrastructure/vector/memory.go`
- LLM:
  - `LLM_PROVIDER=giga` (HTTP gateway adapter)
  - `LLM_PROVIDER=local` (локальный тестовый adapter)

Чтобы добавить нового провайдера, реализуйте соответствующий интерфейс домена и подключите его в `cmd/server/main.go`.

## Размерность эмбеддингов и полная переиндексация (обязательно)

`VECTOR_SIZE` должен строго совпадать с размерностью embedding-модели.  
Для `intfloat/multilingual-e5-large` это `VECTOR_SIZE=1024`.

При смене embedding-модели:

1. Обновите `.env` (`EMBEDDING_MODEL`, `VECTOR_SIZE`).
2. Пересоздайте коллекцию Qdrant:
   ```bash
   make recreate-collection VECTOR_SIZE=1024
   ```
3. Полностью переиндексируйте документы (загрузите заново), иначе семантический поиск будет работать некорректно.

Если данные грузите из Telegram, повторно запустите `scripts/telegram_ingest.go` на экспорт после пересоздания коллекции.

## API endpoints

- `POST /documents`
- `POST /retrieve`
- `POST /chat` (SSE stream)
- `POST /feedback`
- `GET /health`
- `GET /ready`

## Как "обучать" этот RAG (обогащать векторное хранилище)

В RAG под "обучением" обычно понимается **индексация вашей базы знаний**, а не fine-tuning модели.

1. Подготовьте источники: документация, FAQ, wiki, инструкции, тикеты.
2. Отправляйте документы в `POST /documents` с полями:
   - `tenant_id` (изоляция проекта/клиента),
   - `source` (источник),
   - `language`,
   - `tags`,
   - `content`.
3. Сервис асинхронно:
   - режет текст на чанки,
   - строит эмбеддинги,
   - сохраняет векторы + metadata.
4. После индексации используйте `/retrieve` и `/chat` для ответов ассистента.

Пример загрузки:

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

## Как подключить к вашему прод-сервису

Типовой сценарий:

1. Ваш backend хранит контекст tenant/user/session.
2. На вопрос пользователя backend вызывает `POST /chat` (`tenant_id`, `query`, `history`, `max_context_tokens`).
   При необходимости добавляйте `source` и `tags`, чтобы ограничить поиск нужным каналом/тематикой.
3. Backend проксирует SSE-поток в фронтенд/мобильный клиент.
4. После ответа отправляет оценку в `POST /feedback`.

Рекомендации для production:

- **Ingestion pipeline**: триггерите `/documents` из ETL/CDC/jobs при изменениях контента.
- **Tenant isolation**: каждому клиенту/проекту свой `tenant_id`.
- **Observability**: собирайте `/metrics`, логи и трейсы.
- **Provider swap**: меняйте провайдеры через env без изменения бизнес-логики.

## Production checklist

- Включите не-локальные провайдеры:
  - `EMBEDDING_PROVIDER=openrouter` (или ваш embedding-провайдер)
  - `VECTOR_PROVIDER=qdrant`
  - `LLM_PROVIDER=openrouter` (или ваш LLM-провайдер)
- Настройте реальные секреты (`*_API_KEY`) и приватные endpoint-ы.
- Примените миграции PostgreSQL.
- Поставьте TLS/Ingress/API Gateway перед сервисом.
- Добавьте authN/authZ middleware.
- Добавьте лимиты, retries и circuit breaker в вызывающем сервисе.
- Не коммитьте `.env`; если ключ утекал в чат/логи — сразу ротируйте его.

## Фильтрация канала в CLI (`source` / `tags`)

`scripts/chat_cli.sh` умеет ограничивать retrieval по источнику/тегам:

```bash
scripts/chat_cli.sh -t telegram-moskva -s "telegram:@my_channel" -g "news,weather" -q "Что писали про снег?"
```

Важно: передавайте в `-s/--source` ровно то же значение, что использовали при ingestion (для Telegram ingest это `-source`, например `telegram:@my_channel`).

## Дополнительные замечания

- PostgreSQL-адаптер: `internal/infrastructure/repository/postgres.go`.
- SQL-миграция: `migrations/001_init.sql`.
- Redis можно использовать для lock/rate-limit как отдельный инфраструктурный адаптер, не меняя use case-ы.
