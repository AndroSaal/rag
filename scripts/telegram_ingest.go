package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type telegramExport struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	ID       any               `json:"id"`
	Messages []telegramMessage `json:"messages"`
}

type telegramMessage struct {
	ID           int    `json:"id"`
	Type         string `json:"type"`
	Date         string `json:"date"`
	DateUnixTime string `json:"date_unixtime"`
	From         string `json:"from"`
	FromID       string `json:"from_id"`
	Text         any    `json:"text"`
}

type ingestRequest struct {
	TenantID string   `json:"tenant_id"`
	Source   string   `json:"source"`
	Language string   `json:"language"`
	Tags     []string `json:"tags"`
	Content  string   `json:"content"`
}

type ingestResponse struct {
	JobID      string `json:"job_id"`
	DocumentID string `json:"document_id"`
}

var (
	reMessageBlock = regexp.MustCompile(`(?s)<div class="message default clearfix(?: joined)?" id="message(\d+)">(.*?)</div>\s*</div>\s*</div>`)
	reDateTitle    = regexp.MustCompile(`class="pull_right date details" title="([^"]+)"`)
	reFromName     = regexp.MustCompile(`(?s)<div class="from_name">\s*(.*?)\s*</div>`)
	reTextBlock    = regexp.MustCompile(`(?s)<div class="text">\s*(.*?)\s*</div>`)
	reTag          = regexp.MustCompile(`(?s)<[^>]+>`)
	reWhitespace   = regexp.MustCompile(`[ \t]+`)
)

func main() {
	var (
		inputPath      string
		tenantID       string
		source         string
		language       string
		batchSize      int
		requestTimeout int
		dryRun         bool
	)

	flag.StringVar(&inputPath, "input", "", "path to telegram export JSON/HTML file or folder (required)")
	flag.StringVar(&tenantID, "tenant", envOrDefault("RAG_TENANT_ID", ""), "tenant_id for RAG documents")
	flag.StringVar(&source, "source", envOrDefault("RAG_SOURCE", ""), "source value, e.g. telegram:@my_channel")
	flag.StringVar(&language, "language", envOrDefault("RAG_LANGUAGE", "ru"), "language field for documents")
	flag.IntVar(&batchSize, "batch-size", envIntOrDefault("RAG_BATCH_SIZE", 50), "messages per one ingested document")
	flag.IntVar(&requestTimeout, "timeout-sec", envIntOrDefault("RAG_TIMEOUT_SEC", 20), "HTTP timeout per request in seconds")
	flag.BoolVar(&dryRun, "dry-run", false, "print prepared payloads without sending")
	flag.Parse()

	baseURL := strings.TrimRight(envOrDefault("RAG_BASE_URL", "http://localhost:8080"), "/")

	if inputPath == "" {
		exitf("flag -input is required")
	}
	if tenantID == "" {
		exitf("tenant is required: use -tenant or set RAG_TENANT_ID")
	}
	if batchSize <= 0 {
		exitf("batch-size must be > 0")
	}

	exp, err := readTelegramInput(inputPath)
	if err != nil {
		exitf("read export: %v", err)
	}

	if source == "" {
		source = inferSource(exp)
	}

	docs := buildDocuments(exp.Messages, tenantID, source, language, batchSize)
	if len(docs) == 0 {
		fmt.Println("no usable messages found, nothing to ingest")
		return
	}

	fmt.Printf("prepared %d documents from %d telegram messages\n", len(docs), len(exp.Messages))
	if dryRun {
		for i, d := range docs {
			fmt.Printf("\n#%d source=%s tags=%v preview=%q\n", i+1, d.Source, d.Tags, preview(d.Content, 180))
		}
		return
	}

	client := &http.Client{Timeout: time.Duration(requestTimeout) * time.Second}
	for i, d := range docs {
		jobID, docID, err := postDocument(context.Background(), client, baseURL, d)
		if err != nil {
			exitf("ingest failed at document %d/%d: %v", i+1, len(docs), err)
		}
		fmt.Printf("ingested %d/%d job_id=%s document_id=%s\n", i+1, len(docs), jobID, docID)
	}

	fmt.Println("done")
}

func readTelegramInput(path string) (telegramExport, error) {
	info, err := os.Stat(path)
	if err != nil {
		return telegramExport{}, err
	}

	if info.IsDir() {
		msgs, name, err := readTelegramHTMLDir(path)
		if err != nil {
			return telegramExport{}, err
		}
		return telegramExport{Name: name, Type: "channel", Messages: msgs}, nil
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".html" {
		msgs, name, err := readTelegramHTMLFile(path)
		if err != nil {
			return telegramExport{}, err
		}
		return telegramExport{Name: name, Type: "channel", Messages: msgs}, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return telegramExport{}, err
	}

	var exp telegramExport
	if err := json.Unmarshal(raw, &exp); err != nil {
		return telegramExport{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return exp, nil
}

func readTelegramHTMLDir(dir string) ([]telegramMessage, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasPrefix(name, "messages") && strings.HasSuffix(name, ".html") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return nil, "", fmt.Errorf("no messages*.html files found in %s", dir)
	}
	sort.Slice(files, func(i, j int) bool { return htmlPageOrder(files[i]) < htmlPageOrder(files[j]) })

	all := make([]telegramMessage, 0, 1000)
	channelName := ""
	for _, f := range files {
		msgs, name, err := readTelegramHTMLFile(f)
		if err != nil {
			return nil, "", fmt.Errorf("parse %s: %w", f, err)
		}
		if channelName == "" && name != "" {
			channelName = name
		}
		all = append(all, msgs...)
	}
	return all, channelName, nil
}

func readTelegramHTMLFile(path string) ([]telegramMessage, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	s := string(raw)
	name := parseChannelName(s)
	msgs := parseMessagesFromHTML(s)
	return msgs, name, nil
}

func parseChannelName(htmlDoc string) string {
	m := regexp.MustCompile(`(?s)<div class="page_header">.*?<div class="text bold">\s*(.*?)\s*</div>`).FindStringSubmatch(htmlDoc)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(cleanHTMLText(m[1]))
}

func parseMessagesFromHTML(htmlDoc string) []telegramMessage {
	matches := reMessageBlock.FindAllStringSubmatch(htmlDoc, -1)
	out := make([]telegramMessage, 0, len(matches))
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		block := m[2]

		textMatch := reTextBlock.FindStringSubmatch(block)
		if len(textMatch) < 2 {
			continue
		}
		text := strings.TrimSpace(cleanHTMLText(textMatch[1]))
		if text == "" {
			continue
		}

		from := ""
		if fromMatch := reFromName.FindStringSubmatch(block); len(fromMatch) > 1 {
			from = strings.TrimSpace(cleanHTMLText(fromMatch[1]))
		}

		date := ""
		if dateMatch := reDateTitle.FindStringSubmatch(block); len(dateMatch) > 1 {
			date = strings.TrimSpace(cleanHTMLText(dateMatch[1]))
		}

		out = append(out, telegramMessage{
			ID:   id,
			Type: "message",
			Date: date,
			From: from,
			Text: text,
		})
	}
	return out
}

func cleanHTMLText(s string) string {
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = reWhitespace.ReplaceAllString(strings.TrimSpace(lines[i]), " ")
	}
	s = strings.Join(lines, "\n")
	return strings.TrimSpace(s)
}

func htmlPageOrder(path string) int {
	base := strings.ToLower(filepath.Base(path))
	if base == "messages.html" {
		return 1
	}
	num := strings.TrimSuffix(strings.TrimPrefix(base, "messages"), ".html")
	n, err := strconv.Atoi(num)
	if err != nil {
		return 1 << 30
	}
	return n
}

func inferSource(exp telegramExport) string {
	name := strings.TrimSpace(exp.Name)
	if name == "" {
		return "telegram:unknown"
	}
	safe := strings.ReplaceAll(strings.ToLower(name), " ", "_")
	return "telegram:" + safe
}

func buildDocuments(messages []telegramMessage, tenantID, source, language string, batchSize int) []ingestRequest {
	blocks := make([]string, 0, batchSize)
	documents := make([]ingestRequest, 0, len(messages)/batchSize+1)
	firstDate := ""
	lastDate := ""

	for _, m := range messages {
		normalized := normalizeMessage(m)
		if normalized == "" {
			continue
		}
		if firstDate == "" {
			firstDate = msgDate(m)
		}
		lastDate = msgDate(m)

		blocks = append(blocks, normalized)
		if len(blocks) < batchSize {
			continue
		}
		documents = append(documents, makeDoc(tenantID, source, language, firstDate, lastDate, blocks))
		blocks = blocks[:0]
		firstDate = ""
		lastDate = ""
	}

	if len(blocks) > 0 {
		documents = append(documents, makeDoc(tenantID, source, language, firstDate, lastDate, blocks))
	}

	return documents
}

func normalizeMessage(m telegramMessage) string {
	if strings.ToLower(strings.TrimSpace(m.Type)) != "message" {
		return ""
	}
	text := strings.TrimSpace(extractText(m.Text))
	if text == "" {
		return ""
	}
	date := msgDate(m)
	author := strings.TrimSpace(m.From)
	if author == "" {
		author = "unknown"
	}
	return fmt.Sprintf("[msg_id=%d date=%s from=%s]\n%s", m.ID, date, author, text)
}

func extractText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var b strings.Builder
		for _, item := range t {
			b.WriteString(extractText(item))
		}
		return b.String()
	case map[string]any:
		// Telegram Desktop export can contain mixed entities:
		// {"type":"bold","text":"..."} or {"text":"..."}.
		if txt, ok := t["text"]; ok {
			return extractText(txt)
		}
		return ""
	default:
		return ""
	}
}

func msgDate(m telegramMessage) string {
	d := strings.TrimSpace(m.Date)
	if d != "" {
		return d
	}
	return strings.TrimSpace(m.DateUnixTime)
}

func makeDoc(tenantID, source, language, firstDate, lastDate string, blocks []string) ingestRequest {
	content := strings.Join(blocks, "\n\n---\n\n")
	tags := []string{"telegram", "channel_export"}
	if firstDate != "" {
		tags = append(tags, "from:"+firstDate)
	}
	if lastDate != "" {
		tags = append(tags, "to:"+lastDate)
	}
	return ingestRequest{
		TenantID: tenantID,
		Source:   source,
		Language: language,
		Tags:     tags,
		Content:  content,
	}
}

func postDocument(ctx context.Context, client *http.Client, baseURL string, doc ingestRequest) (jobID, documentID string, err error) {
	body, err := json.Marshal(doc)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/documents", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusAccepted {
		return "", "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out ingestResponse
	if len(respBody) == 0 {
		return "", "", errors.New("empty response body")
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}
	return out.JobID, out.DocumentID, nil
}

func preview(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n]) + "..."
}

func envOrDefault(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	return v
}

func envIntOrDefault(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	var out int
	if _, err := fmt.Sscanf(v, "%d", &out); err != nil {
		return def
	}
	return out
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
