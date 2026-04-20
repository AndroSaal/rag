package application

import "github.com/andro/rag/internal/domain"

type IngestDocumentCommand struct {
	DocumentID string
	TenantID   string
	Source     string
	Language   string
	Tags       []string
	Content    string
}

type RetrieveQuery struct {
	TenantID string
	Query    string
	TopK     int
	Filter   domain.RetrievalFilter
}

type RetrieveResult struct {
	QueryID string
	Chunks  []domain.RetrievedChunk
}

type ChatCommand struct {
	TenantID         string
	Query            string
	Source           string
	Tags             []string
	History          []domain.ChatMessage
	TopK             int
	MaxContextTokens int
}

type FeedbackCommand struct {
	TenantID string
	QueryID  string
	Score    int
	Comment  string
}
