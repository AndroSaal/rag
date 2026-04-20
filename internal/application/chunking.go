package application

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/andro/rag/internal/domain"
)

type OverlapChunker struct {
	ChunkSize int
	Overlap   int
}

const telegramMessageSep = "\n\n---\n\n"

func (c OverlapChunker) Split(doc domain.Document) ([]domain.Chunk, error) {
	size := c.ChunkSize
	if size <= 0 {
		size = 800
	}
	overlap := c.Overlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 4
	}

	content := strings.TrimSpace(doc.Content)
	if content == "" {
		return nil, nil
	}

	// Telegram ingest joins messages with "\n\n---\n\n". Splitting on whitespace tokens
	// destroys structured prefixes like "[msg_id=...]" and breaks citations/guardrails.
	units := splitDocUnits(content)
	if len(units) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	idx := 0
	var chunks []domain.Chunk

	var cur []string
	curRunes := 0

	flush := func() {
		if len(cur) == 0 {
			return
		}
		text := strings.Join(cur, telegramMessageSep)
		chunks = append(chunks, domain.Chunk{
			ID:         uuid.NewString(),
			DocumentID: doc.ID,
			TenantID:   doc.TenantID,
			Text:       text,
			Index:      idx,
			Source:     doc.Source,
			Language:   doc.Language,
			Tags:       doc.Tags,
			CreatedAt:  now,
		})
		idx++
	}

	prefixOverlapFrom := func(prev []string) []string {
		if overlap <= 0 || len(prev) == 0 {
			return nil
		}
		var picked []string
		runes := 0
		sep := utf8.RuneCountInString(telegramMessageSep)
		for i := len(prev) - 1; i >= 0; i-- {
			u := prev[i]
			n := utf8.RuneCountInString(u)
			add := n
			if len(picked) > 0 {
				add += sep
			}
			if len(picked) > 0 && runes+add > overlap {
				break
			}
			picked = append(picked, u)
			runes += add
		}
		for i, j := 0, len(picked)-1; i < j; i, j = i+1, j-1 {
			picked[i], picked[j] = picked[j], picked[i]
		}
		return picked
	}

	runeLenJoined := func(parts []string) int {
		if len(parts) == 0 {
			return 0
		}
		total := (len(parts) - 1) * utf8.RuneCountInString(telegramMessageSep)
		for _, p := range parts {
			total += utf8.RuneCountInString(p)
		}
		return total
	}

	for _, u := range units {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		n := utf8.RuneCountInString(u)

		if n > size {
			flush()
			cur = nil
			curRunes = 0

			chunks = append(chunks, domain.Chunk{
				ID:         uuid.NewString(),
				DocumentID: doc.ID,
				TenantID:   doc.TenantID,
				Text:       u,
				Index:      idx,
				Source:     doc.Source,
				Language:   doc.Language,
				Tags:       doc.Tags,
				CreatedAt:  now,
			})
			idx++

			cur = prefixOverlapFrom([]string{u})
			curRunes = runeLenJoined(cur)
			continue
		}

		if curRunes > 0 && curRunes+n > size {
			prev := cur
			flush()
			cur = prefixOverlapFrom(prev)
			curRunes = runeLenJoined(cur)
		}

		cur = append(cur, u)
		curRunes += n
	}
	flush()

	return chunks, nil
}

func splitDocUnits(content string) []string {
	if strings.Contains(content, telegramMessageSep) {
		return strings.Split(content, telegramMessageSep)
	}
	// Generic documents: split on whitespace tokens (legacy behavior).
	toks := strings.Fields(content)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
