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

	log.Printf("Bot v%s started — webhook URL: %s", cfg.Version, cfg.BotWebhookURL)

	tg := telegram.NewClient(cfg.TelegramBotToken)
	stateManager := state.NewManager(cfg.StateFile)
	webhookHandler := handler.NewWebhookHandler(cfg, tg, stateManager)

	// Register webhook with Telegram
	if cfg.BotWebhookURL != "" {
		if err := tg.SetWebhook(cfg.BotWebhookURL); err != nil {
			log.Printf("Warning: failed to set webhook: %v", err)
		} else {
			log.Printf("Webhook registered: %s", cfg.BotWebhookURL)
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
