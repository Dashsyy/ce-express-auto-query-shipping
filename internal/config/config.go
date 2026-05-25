package config

import (
	"os"
	"strconv"
)

type Config struct {
	TelegramBotToken string
	PollInterval     int
	StateFile        string
	MaxCodesPerUser  int
	MinCheckInterval int
	BotWebhookURL    string
	WebhookEnabled   bool
	CacheTTL         int
	Port             string
	Version          string
}

func Load() *Config {
	return &Config{
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		PollInterval:     getEnvInt("POLL_INTERVAL", 3600),
		StateFile:        getEnv("STATE_FILE", "/data/state.json"),
		MaxCodesPerUser:  getEnvInt("MAX_CODES_PER_USER", 10),
		MinCheckInterval: getEnvInt("MIN_CHECK_INTERVAL", 300),
		BotWebhookURL:    getEnv("BOT_WEBHOOK_URL", ""),
		WebhookEnabled:   getEnvBool("WEBHOOK_ENABLED", false),
		CacheTTL:         getEnvInt("CACHE_TTL", 60),
		Port:             getEnv("PORT", "8080"),
		Version:          "2.1.0",
	}
}

func getEnvBool(key string, defaultVal bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultVal
}

func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultVal
}
