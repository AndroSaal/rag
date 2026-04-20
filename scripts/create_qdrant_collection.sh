#!/usr/bin/env sh
set -eu

QDRANT_URL="${QDRANT_URL:-http://localhost:6333}"
QDRANT_COLLECTION="${QDRANT_COLLECTION:-rag_chunks}"
VECTOR_SIZE="${VECTOR_SIZE:-16}"
DISTANCE="${DISTANCE:-Cosine}"

echo "Ensuring Qdrant collection '${QDRANT_COLLECTION}' exists at ${QDRANT_URL}"

if curl -fsS "${QDRANT_URL}/collections/${QDRANT_COLLECTION}" >/dev/null 2>&1; then
  echo "Collection already exists."
  exit 0
fi

curl -fsS -X PUT "${QDRANT_URL}/collections/${QDRANT_COLLECTION}" \
  -H "Content-Type: application/json" \
  -d "{
    \"vectors\": {
      \"size\": ${VECTOR_SIZE},
      \"distance\": \"${DISTANCE}\"
    }
  }" >/dev/null

echo "Collection created."
