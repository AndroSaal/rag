CREATE TABLE IF NOT EXISTS documents (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  source TEXT NOT NULL,
  language TEXT NOT NULL,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS chunks (
  id TEXT PRIMARY KEY,
  document_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  chunk_index INT NOT NULL,
  source TEXT NOT NULL,
  language TEXT NOT NULL,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  text TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_chunks_tenant ON chunks(tenant_id);

CREATE TABLE IF NOT EXISTS feedback (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  query_id TEXT NOT NULL,
  score INT NOT NULL,
  comment TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
