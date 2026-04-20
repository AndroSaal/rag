package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/andro/rag/internal/domain"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) SaveDocument(ctx context.Context, doc domain.Document) error {
	tags, _ := json.Marshal(doc.Tags)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO documents(id, tenant_id, source, language, tags, content, created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)
	`, doc.ID, doc.TenantID, doc.Source, doc.Language, string(tags), doc.Content, doc.CreatedAt)
	return err
}

func (r *PostgresRepository) SaveChunks(ctx context.Context, chunks []domain.Chunk) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range chunks {
		tags, _ := json.Marshal(c.Tags)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chunks(id, document_id, tenant_id, chunk_index, source, language, tags, text, created_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, c.ID, c.DocumentID, c.TenantID, c.Index, c.Source, c.Language, string(tags), c.Text, c.CreatedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *PostgresRepository) GetChunksByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Chunk, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, document_id, tenant_id, chunk_index, source, language, tags, text, created_at
		FROM chunks
		WHERE tenant_id = $1 AND id = ANY($2)
	`, tenantID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Chunk{}
	for rows.Next() {
		var c domain.Chunk
		var tagsJSON string
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.TenantID, &c.Index, &c.Source, &c.Language, &tagsJSON, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &c.Tags)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) SaveFeedback(ctx context.Context, feedback domain.Feedback) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO feedback(id, tenant_id, query_id, score, comment, created_at)
		VALUES($1,$2,$3,$4,$5,$6)
	`, feedback.ID, feedback.TenantID, feedback.QueryID, feedback.Score, feedback.Comment, feedback.CreatedAt)
	return err
}
