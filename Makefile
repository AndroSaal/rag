SHELL := /bin/sh

API_URL ?= http://localhost:8080
QDRANT_URL ?= http://localhost:6333
QDRANT_COLLECTION ?= rag_chunks
VECTOR_SIZE ?= 1024

.PHONY: help up down restart logs ps ensure-collection recreate-collection smoke-test test run

help:
	@echo "Available targets:"
	@echo "  make up                 - build and start docker stack"
	@echo "  make ensure-collection  - create qdrant collection if absent"
	@echo "  make recreate-collection- force recreate qdrant collection"
	@echo "  make smoke-test         - run end-to-end smoke test"
	@echo "  make logs               - tail app logs"
	@echo "  make ps                 - show services status"
	@echo "  make down               - stop stack"
	@echo "  make restart            - restart stack"
	@echo "  make test               - run Go tests"
	@echo "  make run                - run service locally"

up:
	docker compose up --build -d
	$(MAKE) ensure-collection

ensure-collection:
	QDRANT_URL=$(QDRANT_URL) QDRANT_COLLECTION=$(QDRANT_COLLECTION) VECTOR_SIZE=$(VECTOR_SIZE) ./scripts/create_qdrant_collection.sh

recreate-collection:
	QDRANT_URL=$(QDRANT_URL) QDRANT_COLLECTION=$(QDRANT_COLLECTION) VECTOR_SIZE=$(VECTOR_SIZE) ./scripts/recreate_qdrant_collection.sh

smoke-test:
	API_URL=$(API_URL) ./scripts/smoke_test.sh

logs:
	docker compose logs -f app

ps:
	docker compose ps

down:
	docker compose down

restart:
	docker compose down
	docker compose up --build -d
	$(MAKE) ensure-collection

test:
	go test ./...

run:
	go run ./cmd/server
