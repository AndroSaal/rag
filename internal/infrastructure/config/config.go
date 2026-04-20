package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr string

	ProviderLLM       string
	ProviderEmbedding string
	ProviderVectorDB  string

	QdrantURL        string
	QdrantKey        string
	QdrantCollection string

	GigaEmbeddingsURL     string
	GigaEmbeddingsKey     string
	GigaEmbeddingsModel   string
	GigaEmbeddingsVersion string

	EmbeddingURL          string
	EmbeddingKey          string
	EmbeddingModel        string
	OpenRouterHTTPReferer string
	OpenRouterAppName     string

	LLMURL           string
	LLMKey           string
	LLMModel         string
	LLMBackoffModels string

	PostgresDSN string

	RequestTimeout time.Duration
	ChunkSize      int
	ChunkOverlap   int
	DefaultTopK    int
}

func Load() Config {
	cfg := Config{
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		ProviderLLM:           getEnv("LLM_PROVIDER", "giga"),
		ProviderEmbedding:     getEnv("EMBEDDING_PROVIDER", "giga"),
		ProviderVectorDB:      getEnv("VECTOR_PROVIDER", "qdrant"),
		QdrantURL:             getEnv("QDRANT_URL", "http://localhost:6333"),
		QdrantKey:             os.Getenv("QDRANT_API_KEY"),
		QdrantCollection:      getEnv("QDRANT_COLLECTION", "rag_chunks"),
		GigaEmbeddingsURL:     getEnv("GIGA_EMBEDDINGS_URL", "http://localhost:8090/embeddings"),
		GigaEmbeddingsKey:     os.Getenv("GIGA_EMBEDDINGS_API_KEY"),
		GigaEmbeddingsModel:   getEnv("GIGA_EMBEDDINGS_MODEL", "giga-embedding-v1"),
		GigaEmbeddingsVersion: getEnv("GIGA_EMBEDDINGS_VERSION", "v1"),
		EmbeddingURL:          getEnv("EMBEDDING_URL", "https://openrouter.ai/api/v1/embeddings"),
		EmbeddingKey:          firstNonEmpty(os.Getenv("EMBEDDING_API_KEY"), os.Getenv("LLM_API_KEY")),
		EmbeddingModel:        getEnv("EMBEDDING_MODEL", "intfloat/multilingual-e5-large"),
		OpenRouterHTTPReferer: getEnv("OPENROUTER_HTTP_REFERER", "http://localhost:8080"),
		OpenRouterAppName:     getEnv("OPENROUTER_APP_NAME", "generic-rag"),
		LLMURL:                getEnv("LLM_URL", "http://localhost:8090/chat/completions"),
		LLMKey:                os.Getenv("LLM_API_KEY"),
		LLMModel:              getEnv("LLM_MODEL", "giga-chat-pro"),
		LLMBackoffModels:      os.Getenv("LLM_BACKOFF_MODELS"),
		PostgresDSN:           getEnv("DATABASE_URL", "postgres://rag:rag@localhost:5432/rag?sslmode=disable"),
		RequestTimeout:        time.Duration(getInt("REQUEST_TIMEOUT_SECONDS", 30)) * time.Second,
		ChunkSize:             getInt("CHUNK_SIZE", 220),
		ChunkOverlap:          getInt("CHUNK_OVERLAP", 40),
		DefaultTopK:           getInt("DEFAULT_TOP_K", 5),
	}

	if strings.TrimSpace(cfg.LLMBackoffModels) == "" && isOpenRouterLLM(cfg) {
		cfg.LLMBackoffModels = defaultOpenRouterBackoffModels()
	}
	return cfg
}

func isOpenRouterLLM(cfg Config) bool {
	p := strings.ToLower(strings.TrimSpace(cfg.ProviderLLM))
	u := strings.ToLower(cfg.LLMURL)
	return p == "openrouter" || strings.Contains(u, "openrouter.ai/")
}

func defaultOpenRouterBackoffModels() string {
	// Keep concrete model ids first; put OpenRouter router endpoints last as emergency fallbacks.
	return strings.Join([]string{
		"meta-llama/llama-3.2-3b-instruct:free",
		"meta-llama/llama-3.3-70b-instruct:free",
		"arcee-ai/trinity-large-preview:free",
		"cognitivecomputations/dolphin-mistral-24b-venice-edition:free",
		"google/gemma-3-27b-it:free",
		"google/gemma-3-4b-it:free",
		"google/gemma-3n-e2b-it:free",
		"google/gemma-3n-e4b-it:free",
		"google/gemma-4-26b-a4b-it:free",
		"google/gemma-4-31b-it:free",
		"liquid/lfm-2.5-1.2b-instruct:free",
		"liquid/lfm-2.5-1.2b-thinking:free",
		"minimax/minimax-m2.5:free",
		"nousresearch/hermes-3-llama-3.1-405b:free",
		"nvidia/nemotron-3-nano-30b-a3b:free",
		"nvidia/nemotron-3-super-120b-a12b:free",
		"nvidia/nemotron-nano-12b-v2-vl:free",
		"nvidia/nemotron-nano-9b-v2:free",
		"openai/gpt-oss-120b:free",
		"openai/gpt-oss-20b:free",
		"qwen/qwen3-coder:free",
		"qwen/qwen3-next-80b-a3b-instruct:free",
		"z-ai/glm-4.5-air:free",
		"openrouter/auto",
		"openrouter/free",
	}, ",")
}

func (c Config) Validate() error {
	var issues []string

	if requiresOpenRouterMeta(c.LLMURL) {
		if strings.TrimSpace(c.LLMKey) == "" {
			issues = append(issues, "LLM_API_KEY is required for OpenRouter LLM endpoint")
		}
		if strings.TrimSpace(c.OpenRouterHTTPReferer) == "" {
			issues = append(issues, "OPENROUTER_HTTP_REFERER is required for OpenRouter LLM endpoint")
		}
		if strings.TrimSpace(c.OpenRouterAppName) == "" {
			issues = append(issues, "OPENROUTER_APP_NAME is required for OpenRouter LLM endpoint")
		}
	}

	if requiresOpenRouterMeta(c.EmbeddingURL) {
		if strings.TrimSpace(c.EmbeddingKey) == "" {
			issues = append(issues, "EMBEDDING_API_KEY (or LLM_API_KEY fallback) is required for OpenRouter embeddings endpoint")
		}
		if strings.TrimSpace(c.OpenRouterHTTPReferer) == "" {
			issues = append(issues, "OPENROUTER_HTTP_REFERER is required for OpenRouter embeddings endpoint")
		}
		if strings.TrimSpace(c.OpenRouterAppName) == "" {
			issues = append(issues, "OPENROUTER_APP_NAME is required for OpenRouter embeddings endpoint")
		}
	}

	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("config validation failed: %s", strings.Join(issues, "; "))
}

func requiresOpenRouterMeta(url string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(url)), "openrouter.ai/")
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
