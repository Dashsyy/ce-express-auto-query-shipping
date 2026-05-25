package main

import (
	"fmt"
	"log"
	"time"

	"github.com/dashsyy/ce-tracker/internal/config"
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

				// Build alert message
				msg := buildAlertMessage(code, result, &prev)
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

func buildAlertMessage(code string, result *tracker.TrackingResult, prevState *state.CodeState) string {
	prevLabel := tracker.GetStatusLabel(prevState.ShipmentStatus)
	newLabel := tracker.GetStatusLabel(result.ShipmentStatus)

	msg := "🔔 <b>STATUS UPDATE!</b>\n"
	msg += fmt.Sprintf("Tracking: <code>%s</code>\n", code)
	msg += fmt.Sprintf("Status changed: <b>%s</b> → <b>%s</b>\n", prevLabel, newLabel)

	if result.Destination != "" {
		msg += fmt.Sprintf("Destination: %s\n", result.Destination)
	}
	if result.Weight > 0 {
		msg += fmt.Sprintf("Weight: %.2f kg\n", result.Weight)
	}

	if len(result.Events) > 0 {
		msg += "\n<b>Latest events:</b>\n"
		start := len(result.Events) - 4
		if start < 0 {
			start = 0
		}
		for _, ev := range result.Events[start:] {
			t := ev.Time
			desc := ev.Description
			shop := ev.Shop
			if shop != "" {
				msg += fmt.Sprintf("  %s [%s]: %s\n", t, shop, desc)
			} else {
				msg += fmt.Sprintf("  %s: %s\n", t, desc)
			}
		}
	}

	if !result.Delivered && result.ShipmentStatus == "40" && result.CourierName != "" {
		msg += fmt.Sprintf("\nCourier: <b>%s</b>  📞 %s\n", result.CourierName, result.CourierMobile)
	}

	return msg
}
