package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dashsyy/ce-tracker/internal/config"
	"github.com/dashsyy/ce-tracker/internal/ratelimit"
	"github.com/dashsyy/ce-tracker/internal/state"
	"github.com/dashsyy/ce-tracker/internal/telegram"
	"github.com/dashsyy/ce-tracker/internal/tracker"
)

type WebhookHandler struct {
	cfg    *config.Config
	tg     *telegram.Client
	state  *state.Manager
}

func NewWebhookHandler(cfg *config.Config, tg *telegram.Client, stateManager *state.Manager) *WebhookHandler {
	return &WebhookHandler{
		cfg:   cfg,
		tg:    tg,
		state: stateManager,
	}
}

func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	var update tgbotapi.Update
	if err := c.BindJSON(&update); err != nil {
		log.Printf("Failed to parse webhook: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	chatIDStr := strconv.FormatInt(update.FromChat().ID, 10)

	if update.Message != nil {
		if update.Message.IsCommand() {
			h.handleCommand(update.Message.Command(), chatIDStr)
		} else if update.Message.Text != "" {
			h.handleText(update.Message.Text, chatIDStr)
		}
	} else if update.CallbackQuery != nil {
		h.handleCallback(update.CallbackQuery, chatIDStr)
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *WebhookHandler) handleCommand(command, chatID string) {
	if command == "start" {
		h.handleStart(chatID)
	}
}

func (h *WebhookHandler) handleStart(chatID string) {
	appState, err := h.state.Load()
	if err != nil {
		log.Printf("Failed to load state: %v", err)
		return
	}

	chat := h.state.GetChat(appState, chatID)
	versionInfo := "\n\n<i>v" + h.cfg.Version + "</i>"

	if len(chat.Codes) > 0 {
		keyboard := buildCodesKeyboard(chat.Codes)
		text := "👋 Welcome back!\n\nTap a previous code to re-check, or send a new tracking code:" + versionInfo
		h.tg.SendMessageWithKeyboard(chatID, text, "HTML", keyboard)
	} else {
		text := "👋 Welcome to <b>CE Express Tracker</b>!\n\n" +
			"Send me a tracking code (e.g. <code>TBKH000649682</code>) and I'll check its " +
			"status and alert you automatically whenever it changes." + versionInfo
		h.tg.SendMessage(chatID, text, "HTML")
	}
}

func (h *WebhookHandler) handleText(code, chatID string) {
	appState, err := h.state.Load()
	if err != nil {
		log.Printf("Failed to load state: %v", err)
		h.tg.SendMessage(chatID, "❌ Error loading state", "HTML")
		return
	}

	code = uppercase(code)
	chat := h.state.GetChat(appState, chatID)

	// Rate limit: max codes per user
	if !contains(chat.Codes, code) {
		ok, errMsg := ratelimit.CanAddCode(appState, chatID, h.cfg.MaxCodesPerUser)
		if !ok {
			h.tg.SendMessage(chatID, errMsg, "HTML")
			return
		}
	}

	// Rate limit: check interval
	ok, errMsg := ratelimit.CanCheckCode(appState, chatID, code, h.cfg.MinCheckInterval)
	if !ok {
		h.tg.SendMessage(chatID, errMsg, "HTML")
		return
	}

	h.tg.SendMessage(chatID, "🔍 Checking <code>"+code+"</code>…", "HTML")

	result, err := tracker.Fetch(code)
	if err != nil {
		h.tg.SendMessage(chatID, "❌ Could not fetch <code>"+code+"</code>: "+err.Error(), "HTML")
		return
	}

	msg := buildTrackingMessage(code, result, false, nil, h.cfg.Version)
	h.tg.SendMessage(chatID, msg, "HTML")

	// Update state
	if !contains(chat.Codes, code) {
		chat.Codes = append(chat.Codes, code)
	}
	codeState := state.CodeState{
		ShipmentStatus: result.ShipmentStatus,
		LastEventCode:  result.LastEventCode,
		LastEventTime:  result.LastEventTime,
		Delivered:      result.Delivered,
	}
	h.state.UpdateCodeState(appState, chatID, code, codeState)
	chat.CodeStates[code] = codeState
	h.state.SetChat(appState, chatID, chat)
	h.state.Save(appState)
}

func (h *WebhookHandler) handleCallback(cb *tgbotapi.CallbackQuery, chatID string) {
	if len(cb.Data) < 6 || cb.Data[:6] != "track:" {
		return
	}

	code := cb.Data[6:]
	appState, err := h.state.Load()
	if err != nil {
		log.Printf("Failed to load state: %v", err)
		return
	}

	// Rate limit: check interval
	ok, errMsg := ratelimit.CanCheckCode(appState, chatID, code, h.cfg.MinCheckInterval)
	if !ok {
		h.tg.SendMessage(chatID, errMsg, "HTML")
		return
	}

	h.tg.SendMessage(chatID, "🔍 Checking <code>"+code+"</code>…", "HTML")

	result, err := tracker.Fetch(code)
	if err != nil {
		h.tg.SendMessage(chatID, "❌ Could not fetch <code>"+code+"</code>: "+err.Error(), "HTML")
		return
	}

	msg := buildTrackingMessage(code, result, false, nil, h.cfg.Version)
	h.tg.SendMessage(chatID, msg, "HTML")

	// Update state
	codeState := state.CodeState{
		ShipmentStatus: result.ShipmentStatus,
		LastEventCode:  result.LastEventCode,
		LastEventTime:  result.LastEventTime,
		Delivered:      result.Delivered,
	}
	h.state.UpdateCodeState(appState, chatID, code, codeState)
	h.state.Save(appState)
}

func buildCodesKeyboard(codes []string) [][]telegram.InlineKeyboardButton {
	var rows [][]telegram.InlineKeyboardButton
	for i := 0; i < len(codes); i += 2 {
		var row []telegram.InlineKeyboardButton
		row = append(row, telegram.InlineKeyboardButton{
			Text:         codes[i],
			CallbackData: "track:" + codes[i],
		})
		if i+1 < len(codes) {
			row = append(row, telegram.InlineKeyboardButton{
				Text:         codes[i+1],
				CallbackData: "track:" + codes[i+1],
			})
		}
		rows = append(rows, row)
	}
	return rows
}

func buildTrackingMessage(code string, result *tracker.TrackingResult, isAlert bool, prevState *state.CodeState, version string) string {
	label := tracker.GetStatusLabel(result.ShipmentStatus)
	var msg string

	if isAlert && prevState != nil {
		prevLabel := tracker.GetStatusLabel(prevState.ShipmentStatus)
		msg = "🔔 <b>STATUS UPDATE!</b>\n"
		msg += "Tracking: <code>" + code + "</code>\n"
		msg += "Status changed: <b>" + prevLabel + "</b> → <b>" + label + "</b>\n"
	} else {
		if result.Delivered {
			msg = "✅ <b>PACKAGE DELIVERED!</b>\n"
		} else {
			msg = "📦 <b>CE Express Tracker</b>\n"
		}
		msg += "Tracking: <code>" + code + "</code>\n"
		msg += "Status: <b>" + label + "</b>\n"
	}

	if result.Destination != "" {
		msg += "Destination: " + result.Destination + "\n"
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
				msg += "  " + t + " [" + shop + "]: " + desc + "\n"
			} else {
				msg += "  " + t + ": " + desc + "\n"
			}
		}
	}

	if !result.Delivered && result.ShipmentStatus == "40" && result.CourierName != "" {
		msg += "\nCourier: <b>" + result.CourierName + "</b>  📞 " + result.CourierMobile + "\n"
	}

	return msg
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func uppercase(s string) string {
	result := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			result[i] = c - 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}
