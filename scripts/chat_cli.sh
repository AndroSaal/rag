#!/usr/bin/env sh
set -eu

API_URL="${API_URL:-http://localhost:8080}"
TENANT_ID="${TENANT_ID:-telegram-moskva}"
TOP_K="${TOP_K:-5}"
MAX_CONTEXT_TOKENS="${MAX_CONTEXT_TOKENS:-1500}"
SOURCE="${CHAT_SOURCE:-}"
TAGS_CSV="${CHAT_TAGS:-}"
QUESTION=""

usage() {
  cat <<EOF
Usage:
  scripts/chat_cli.sh [options]

Options:
  -q, --question TEXT        Ask one question and exit
  -t, --tenant ID            tenant_id (default: ${TENANT_ID})
  -a, --api-url URL          API base URL (default: ${API_URL})
  -s, --source TEXT          retrieval filter: source (optional)
  -g, --tags CSV             retrieval filter: comma-separated tags (optional)
  -k, --top-k N              top_k for retrieval (default: ${TOP_K})
  -m, --max-context N        max_context_tokens (default: ${MAX_CONTEXT_TOKENS})
  -h, --help                 Show this help

Env alternatives:
  API_URL, TENANT_ID, TOP_K, MAX_CONTEXT_TOKENS, CHAT_SOURCE, CHAT_TAGS

Examples:
  scripts/chat_cli.sh -q "Что писали про снегопад в Москве?"
  TENANT_ID=demo scripts/chat_cli.sh
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    -q|--question)
      QUESTION="$2"
      shift 2
      ;;
    -t|--tenant)
      TENANT_ID="$2"
      shift 2
      ;;
    -a|--api-url)
      API_URL="$2"
      shift 2
      ;;
    -k|--top-k)
      TOP_K="$2"
      shift 2
      ;;
    -m|--max-context)
      MAX_CONTEXT_TOKENS="$2"
      shift 2
      ;;
    -s|--source)
      SOURCE="$2"
      shift 2
      ;;
    -g|--tags)
      TAGS_CSV="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

ask_once() {
  q="$1"
  if [ -z "$q" ]; then
    return 0
  fi

  body="$(QUESTION="$q" TENANT_ID="$TENANT_ID" TOP_K="$TOP_K" MAX_CONTEXT_TOKENS="$MAX_CONTEXT_TOKENS" SOURCE="$SOURCE" TAGS_CSV="$TAGS_CSV" python3 - <<'PY'
import json
import os

tags = []
raw = os.environ.get("TAGS_CSV", "").strip()
if raw:
    tags = [t.strip() for t in raw.split(",") if t.strip()]

payload = {
    "tenant_id": os.environ["TENANT_ID"],
    "query": os.environ["QUESTION"],
    "top_k": int(os.environ["TOP_K"]),
    "max_context_tokens": int(os.environ["MAX_CONTEXT_TOKENS"]),
    "history": [],
}
src = os.environ.get("SOURCE", "").strip()
if src:
    payload["source"] = src
if tags:
    payload["tags"] = tags

print(json.dumps(payload, ensure_ascii=False))
PY
)"

  printf "\nQ: %s\nA: " "$q"
  curl -fsS -N -X POST "${API_URL%/}/chat" \
    -H "Content-Type: application/json" \
    -d "$body" | python3 -c 'import json,sys
for raw in sys.stdin:
    line = raw.strip()
    if not line or not line.startswith("data: "):
        continue
    payload = line[6:]
    try:
        obj = json.loads(payload)
    except Exception:
        continue
    text = obj.get("Text", "")
    if text:
        print(text, end="", flush=True)
print()'
}

if [ -n "$QUESTION" ]; then
  ask_once "$QUESTION"
  exit 0
fi

echo "RAG chat CLI"
echo "API_URL=${API_URL}"
echo "TENANT_ID=${TENANT_ID}"
echo "Type question and press Enter. Type 'exit' to quit."

while true; do
  printf "\n> "
  IFS= read -r q || break
  case "$q" in
    "" )
      continue
      ;;
    exit|quit)
      break
      ;;
  esac
  ask_once "$q"
done

