#!/usr/bin/env sh
set -eu

API_URL="${API_URL:-http://localhost:8080}"
TENANT_ID="${TENANT_ID:-demo-tenant}"

echo "1) Health checks"
curl -fsS "${API_URL}/health" >/dev/null
curl -fsS "${API_URL}/ready" >/dev/null
echo "OK"

echo "2) Ingest document"
INGEST_RESP="$(curl -fsS -X POST "${API_URL}/documents" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\":\"${TENANT_ID}\",
    \"source\":\"smoke\",
    \"language\":\"ru\",
    \"tags\":[\"smoke\",\"rag\"],
    \"content\":\"RAG система использует чанкинг, эмбеддинги и векторный поиск для ответа на вопросы пользователя.\"
  }")"
echo "${INGEST_RESP}"

echo "3) Wait async ingestion"
sleep 2

echo "4) Retrieve"
RETRIEVE_RESP="$(curl -fsS -X POST "${API_URL}/retrieve" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\":\"${TENANT_ID}\",
    \"query\":\"Как работает RAG?\",
    \"top_k\":3
  }")"
echo "${RETRIEVE_RESP}"

echo "5) Chat SSE (first lines)"
curl -fsS -N -X POST "${API_URL}/chat" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\":\"${TENANT_ID}\",
    \"query\":\"Объясни как работает RAG\",
    \"top_k\":3,
    \"max_context_tokens\":700
  }" | awk 'NR<=8 {print} NR==9 {exit}'

echo "Smoke test completed."
