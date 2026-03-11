package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppName         string
	Domain          string
	HTTPAddr        string
	PublicBaseURL   string
	MaxBodyBytes    int64
	RoundDuration   int
	RoomCodeLength  int
	MaxChatMessages int
}

func FromEnv() Config {
	return Config{
		AppName:         env("APP_NAME", "Мишаня шаманит"),
		Domain:          env("APP_DOMAIN", "miha.vovengo.com"),
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		PublicBaseURL:   env("PUBLIC_BASE_URL", "http://localhost:8080"),
		MaxBodyBytes:    int64(envInt("MAX_BODY_BYTES", 1<<20)),
		RoundDuration:   envInt("ROUND_DURATION_SECONDS", 90),
		RoomCodeLength:  envInt("ROOM_CODE_LENGTH", 6),
		MaxChatMessages: envInt("MAX_CHAT_MESSAGES", 80),
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
