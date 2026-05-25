package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dashsyy/ce-tracker/internal/config"
	"github.com/dashsyy/ce-tracker/internal/format"
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
		log.Printf("[WEBHOOK] ✗ parse error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	chatIDStr := strconv.FormatInt(update.FromChat().ID, 10)
	username := ""
	if update.SentFrom() != nil {
		username = update.SentFrom().UserName
		if username == "" {
			username = update.SentFrom().FirstName
		}
	}

	if update.Message != nil {
		if update.Message.IsCommand() {
			log.Printf("[USER] chat=%s user=@%s cmd=/%s", chatIDStr, username, update.Message.Command())
			h.handleCommand(update.Message.Command(), chatIDStr)
		} else if update.Message.Text != "" {
			log.Printf("[USER] chat=%s user=@%s text=%q", chatIDStr, username, update.Message.Text)
			h.handleText(update.Message.Text, chatIDStr)
		}
	} else if update.CallbackQuery != nil {
		log.Printf("[USER] chat=%s user=@%s button=%s", chatIDStr, username, update.CallbackQuery.Data)
		h.handleCallback(update.CallbackQuery, chatIDStr)
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *WebhookHandler) handleCommand(command, chatID string) {
	switch command {
	case "start":
		h.handleStart(chatID)
	case "list":
		h.handleList(chatID)
	case "delete", "remove":
		h.handleDelete(chatID)
	case "help":
		h.handleHelp(chatID)
	default:
		h.tg.SendMessage(chatID, "❓ Unknown command. Try /help", "HTML")
	}
}

func (h *WebhookHandler) handleHelp(chatID string) {
	text := "<b>📚 Commands</b>\n" +
		"━━━━━━━━━━━━━━━━\n" +
		"/start — Show your tracked codes\n" +
		"/list — List all tracked codes\n" +
		"/delete — Remove tracked codes\n" +
		"/help — Show this help\n\n" +
		"<b>📦 Adding codes</b>\n" +
		"Just send any tracking code as a message (e.g. <code>TBKH000649682</code>)"
	h.tg.SendMessage(chatID, text, "HTML")
}

func (h *WebhookHandler) handleList(chatID string) {
	appState, err := h.state.Load()
	if err != nil {
		log.Printf("Failed to load state: %v", err)
		return
	}
	chat := h.state.GetChat(appState, chatID)

	if len(chat.Codes) == 0 {
		h.tg.SendMessage(chatID, "📭 No tracked codes yet. Send a tracking code to add one.", "HTML")
		return
	}

	text := fmt.Sprintf("📋 <b>Tracked codes (%d/%d)</b>\n━━━━━━━━━━━━━━━━\n", len(chat.Codes), h.cfg.MaxCodesPerUser)
	for _, code := range chat.Codes {
		status := "?"
		if cs, ok := chat.CodeStates[code]; ok {
			status = tracker.GetStatusLabel(cs.ShipmentStatus)
		}
		text += fmt.Sprintf("• <code>%s</code> — %s\n", code, status)
	}
	h.tg.SendMessage(chatID, text, "HTML")
}

func (h *WebhookHandler) handleDelete(chatID string) {
	appState, err := h.state.Load()
	if err != nil {
		log.Printf("Failed to load state: %v", err)
		return
	}
	chat := h.state.GetChat(appState, chatID)

	if len(chat.Codes) == 0 {
		h.tg.SendMessage(chatID, "📭 No codes to delete.", "HTML")
		return
	}

	keyboard := buildDeleteKeyboard(chat.Codes)
	h.tg.SendMessageWithKeyboard(chatID, "🗑 <b>Tap a code to delete it:</b>", "HTML", keyboard)
}

func buildDeleteKeyboard(codes []string) [][]telegram.InlineKeyboardButton {
	var rows [][]telegram.InlineKeyboardButton
	for _, code := range codes {
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: "🗑 " + code, CallbackData: "del:" + code},
		})
	}
	return rows
}

func (h *WebhookHandler) deleteCode(chatID, code string) {
	appState, err := h.state.Load()
	if err != nil {
		log.Printf("Failed to load state: %v", err)
		return
	}
	chat := h.state.GetChat(appState, chatID)

	// Remove code from slice
	newCodes := make([]string, 0, len(chat.Codes))
	found := false
	for _, c := range chat.Codes {
		if c == code {
			found = true
			continue
		}
		newCodes = append(newCodes, c)
	}

	if !found {
		h.tg.SendMessage(chatID, fmt.Sprintf("❌ <code>%s</code> not found in your codes.", code), "HTML")
		return
	}

	chat.Codes = newCodes
	delete(chat.CodeStates, code)
	h.state.SetChat(appState, chatID, chat)
	h.state.Save(appState)

	log.Printf("[USER] chat=%s deleted code=%s (remaining: %d)", chatID, code, len(newCodes))
	h.tg.SendMessage(chatID, fmt.Sprintf("✅ Deleted <code>%s</code>. You now have %d/%d codes.", code, len(newCodes), h.cfg.MaxCodesPerUser), "HTML")
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
		text := fmt.Sprintf("👋 Welcome back!\n\nYou have %d/%d codes.\nTap to re-check, or send a new code.\n\n<i>Commands:</i> /list /delete /help", len(chat.Codes), h.cfg.MaxCodesPerUser) + versionInfo
		h.tg.SendMessageWithKeyboard(chatID, text, "HTML", keyboard)
	} else {
		text := "👋 Welcome to <b>CE Express Tracker</b>!\n\n" +
			"Send me a tracking code (e.g. <code>TBKH000649682</code>) and I'll check its " +
			"status and alert you automatically whenever it changes.\n\n" +
			"<i>Commands:</i> /list /delete /help" + versionInfo
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

	msg := format.TrackingMessage(code, result, nil)
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
	// Handle delete button
	if len(cb.Data) > 4 && cb.Data[:4] == "del:" {
		code := cb.Data[4:]
		h.deleteCode(chatID, code)
		return
	}

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

	msg := format.TrackingMessage(code, result, nil)
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
