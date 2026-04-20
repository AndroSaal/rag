package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/andro/rag/internal/domain"
)

type HTTPGateway struct {
	url            string
	apiKey         string
	model          string
	fallbackModels []string
	siteURL        string
	appName        string
	client         *http.Client
}

func NewHTTPGateway(url, key, model, backoffModelsCSV, siteURL, appName string, timeout time.Duration) *HTTPGateway {
	return &HTTPGateway{
		url: url, apiKey: key, model: model,
		fallbackModels: parseModelCSV(backoffModelsCSV),
		siteURL:        siteURL,
		appName:        appName,
		client:         &http.Client{Timeout: timeout},
	}
}

func (g *HTTPGateway) StreamCompletion(ctx context.Context, req domain.LLMRequest) (<-chan domain.LLMStreamChunk, error) {
	models := uniqueModels(append([]string{g.model}, g.fallbackModels...))
	var lastErr error
	for _, m := range models {
		ch, err := g.streamModel(ctx, m, req)
		if err == nil {
			return ch, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no llm models configured")
	}
	return nil, lastErr
}

func parseModelCSV(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func uniqueModels(models []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(models))
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

func (g *HTTPGateway) streamModel(ctx context.Context, model string, req domain.LLMRequest) (<-chan domain.LLMStreamChunk, error) {
	body := map[string]any{"model": model, "messages": req.Messages, "stream": true}
	b, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if g.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	}
	if g.siteURL != "" {
		httpReq.Header.Set("HTTP-Referer", g.siteURL)
	}
	if g.appName != "" {
		httpReq.Header.Set("X-Title", g.appName)
	}
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("llm model=%s status=%d body=%s", model, resp.StatusCode, string(raw))
	}
	out := make(chan domain.LLMStreamChunk, 32)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "data:") {
				line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
			if line == "[DONE]" {
				out <- domain.LLMStreamChunk{Done: true}
				return
			}
			text, done, usage, ok := parseStreamChunk(line)
			if !ok {
				continue
			}
			out <- domain.LLMStreamChunk{Text: text, Done: done, Usage: usage}
		}
		out <- domain.LLMStreamChunk{Done: true}
	}()
	return out, nil
}

func parseStreamChunk(line string) (text string, done bool, usage *domain.LLMUsage, ok bool) {
	// Native gateway-compatible format.
	var direct struct {
		Text  string           `json:"text"`
		Done  bool             `json:"done"`
		Usage *domain.LLMUsage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(line), &direct); err == nil && (direct.Text != "" || direct.Done || direct.Usage != nil) {
		return direct.Text, direct.Done, direct.Usage, true
	}

	// OpenAI-compatible streaming format (used by OpenRouter and many providers).
	var openai struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(line), &openai); err != nil {
		return "", false, nil, false
	}

	if openai.Usage != nil {
		usage = &domain.LLMUsage{
			InputTokens:  openai.Usage.PromptTokens,
			OutputTokens: openai.Usage.CompletionTokens,
		}
	}
	if len(openai.Choices) == 0 {
		return "", false, usage, usage != nil
	}

	ch := openai.Choices[0]
	text = ch.Delta.Content
	if text == "" {
		text = ch.Message.Content
	}
	done = ch.FinishReason != nil && *ch.FinishReason != ""
	return text, done, usage, text != "" || done || usage != nil
}

type LocalGateway struct{}

func NewLocalGateway() *LocalGateway { return &LocalGateway{} }

func (g *LocalGateway) StreamCompletion(ctx context.Context, req domain.LLMRequest) (<-chan domain.LLMStreamChunk, error) {
	ch := make(chan domain.LLMStreamChunk, 8)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
			ch <- domain.LLMStreamChunk{Text: "request cancelled", Done: true}
			return
		default:
		}
		msg := "Локальный ответ: " + req.Messages[len(req.Messages)-1].Content
		for _, part := range strings.Split(msg, " ") {
			ch <- domain.LLMStreamChunk{Text: part + " "}
			time.Sleep(10 * time.Millisecond)
		}
		ch <- domain.LLMStreamChunk{Done: true, Usage: &domain.LLMUsage{InputTokens: 100, OutputTokens: 40}}
	}()
	return ch, nil
}
