package config

import (
	"os"
	"strings"
)

type Config struct {
	AppName               string
	Domain                string
	HTTPAddr              string
	PublicBaseURL         string
	GeneratorProvider     string
	OpenAIBaseURL         string
	OpenAIAPIKey          string
	OpenAIModel           string
	OpenAIImageSize       string
	StorageDir            string
	MaxUploadBytes        int64
	SystemPrompt          string
}

func FromEnv() Config {
	cfg := Config{
		AppName:           env("APP_NAME", "Мишаня шаманит"),
		Domain:            env("APP_DOMAIN", "miha.vovengo.com"),
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		PublicBaseURL:     env("PUBLIC_BASE_URL", "http://localhost:8080"),
		GeneratorProvider: strings.ToLower(env("GEN_PROVIDER", "mock")),
		OpenAIBaseURL:     env("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:      env("OPENAI_API_KEY", ""),
		OpenAIModel:       env("OPENAI_IMAGE_MODEL", "gpt-image-1"),
		OpenAIImageSize:   env("OPENAI_IMAGE_SIZE", "1024x1024"),
		StorageDir:        env("STORAGE_DIR", "data/jobs"),
		MaxUploadBytes:    12 << 20,
		SystemPrompt: env("SYSTEM_PROMPT", "Absurd cyberpunk village scene, neon mud, rustic techno-magic, keep the user's sketch composition and subjects recognizable, enhance details, cinematic lighting, playful but not grotesque."),
	}
	return cfg
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
