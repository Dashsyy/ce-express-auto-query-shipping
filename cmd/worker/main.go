package main

import (
	"log"
	"time"

	"github.com/dashsyy/ce-tracker/internal/config"
	"github.com/dashsyy/ce-tracker/internal/format"
	"github.com/dashsyy/ce-tracker/internal/state"
	"github.com/dashsyy/ce-tracker/internal/telegram"
	"github.com/dashsyy/ce-tracker/internal/tracker"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	cfg := config.Load()

	if cfg.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	log.Printf("Worker v%s started — poll interval: %ds", cfg.Version, cfg.PollInterval)

	tg := telegram.NewClient(cfg.TelegramBotToken)
	stateManager := state.NewManager(cfg.StateFile)

	// Start polling loop
	ticker := time.NewTicker(time.Duration(cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	// Run once on startup
	pollAll(stateManager, tg)

	for range ticker.C {
		pollAll(stateManager, tg)
	}
}

func pollAll(stateManager *state.Manager, tg *telegram.Client) {
	appState, err := stateManager.Load()
	if err != nil {
		log.Printf("Failed to load state: %v", err)
		return
	}

	log.Println("Background poll starting")

	for chatIDStr, chat := range appState.Chats {
		for _, code := range chat.Codes {
			prev := chat.CodeStates[code]

			// Bypass cache — worker needs fresh data to detect changes
			tracker.InvalidateCache(code)
			result, err := tracker.Fetch(code)
			if err != nil {
				log.Printf("[%s] fetch error: %v", code, err)
				continue
			}

			// Check if status changed
			changed := result.ShipmentStatus != prev.ShipmentStatus ||
				result.LastEventCode != prev.LastEventCode

			if changed {
				log.Printf("[%s] change detected — notifying chat %s", code, chatIDStr)

				// Build alert message using shared formatter
				msg := format.TrackingMessage(code, result, &prev)
				if err := tg.SendMessage(chatIDStr, msg, "HTML"); err != nil {
					log.Printf("[%s] failed to notify %s: %v", code, chatIDStr, err)
				}
			}

			// Update state
			newCodeState := state.CodeState{
				ShipmentStatus: result.ShipmentStatus,
				LastEventCode:  result.LastEventCode,
				LastEventTime:  result.LastEventTime,
				Delivered:      result.Delivered,
				CheckedAt:      time.Now().UTC().Format(time.RFC3339),
			}
			stateManager.UpdateCodeState(appState, chatIDStr, code, newCodeState)
		}
	}

	if err := stateManager.Save(appState); err != nil {
		log.Printf("Failed to save state: %v", err)
	}

	log.Println("Background poll done")
}

