#!/usr/bin/env sh
set -eu

QDRANT_URL="${QDRANT_URL:-http://localhost:6333}"
QDRANT_COLLECTION="${QDRANT_COLLECTION:-rag_chunks}"
VECTOR_SIZE="${VECTOR_SIZE:-1024}"
DISTANCE="${DISTANCE:-Cosine}"

echo "Recreating Qdrant collection '${QDRANT_COLLECTION}' at ${QDRANT_URL} (size=${VECTOR_SIZE}, distance=${DISTANCE})"

curl -fsS -X DELETE "${QDRANT_URL}/collections/${QDRANT_COLLECTION}" >/dev/null 2>&1 || true

curl -fsS -X PUT "${QDRANT_URL}/collections/${QDRANT_COLLECTION}" \
  -H "Content-Type: application/json" \
  -d "{
    \"vectors\": {
      \"size\": ${VECTOR_SIZE},
      \"distance\": \"${DISTANCE}\"
    }
  }" >/dev/null

echo "Collection recreated."
