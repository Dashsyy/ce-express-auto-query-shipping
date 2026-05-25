package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dashsyy/ce-tracker/internal/config"
	"github.com/dashsyy/ce-tracker/internal/handler"
	"github.com/dashsyy/ce-tracker/internal/state"
	"github.com/dashsyy/ce-tracker/internal/telegram"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	cfg := config.Load()

	if cfg.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	log.Printf("Bot v%s started — webhook enabled: %v", cfg.Version, cfg.WebhookEnabled)

	tg := telegram.NewClient(cfg.TelegramBotToken)
	stateManager := state.NewManager(cfg.StateFile)
	webhookHandler := handler.NewWebhookHandler(cfg, tg, stateManager)

	// Webhook kill switch logic
	if cfg.WebhookEnabled {
		if cfg.BotWebhookURL == "" {
			log.Println("⚠️  WEBHOOK_ENABLED=true but BOT_WEBHOOK_URL is empty — skipping webhook registration")
		} else {
			if err := tg.SetWebhook(cfg.BotWebhookURL); err != nil {
				log.Printf("Warning: failed to set webhook: %v", err)
			} else {
				log.Printf("✅ Webhook registered: %s", cfg.BotWebhookURL)
			}
		}
	} else {
		// Webhook disabled — clean up any stale webhook from previous deploys
		if err := tg.DeleteWebhook(); err != nil {
			log.Printf("Warning: failed to delete webhook: %v", err)
		} else {
			log.Println("🔒 Webhook DISABLED — cleared any existing webhook. Set WEBHOOK_ENABLED=true to enable.")
		}
	}

	// Setup Gin router
	router := gin.Default()

	router.POST("/webhook", webhookHandler.HandleWebhook)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": cfg.Version,
		})
	})

	log.Printf("Bot server listening on port %s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
