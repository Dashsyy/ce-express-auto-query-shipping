package format

import (
	"fmt"
	"strings"
	"time"

	"github.com/dashsyy/ce-tracker/internal/state"
	"github.com/dashsyy/ce-tracker/internal/tracker"
)

const separator = "━━━━━━━━━━━━━━━━"

// TrackingMessage builds a clean, minimal message for displaying tracking info.
// If prevState is not nil, formats as a status change alert.
func TrackingMessage(code string, result *tracker.TrackingResult, prevState *state.CodeState) string {
	var b strings.Builder

	// Header
	if prevState != nil {
		prevLabel := tracker.GetStatusLabel(prevState.ShipmentStatus)
		newLabel := tracker.GetStatusLabel(result.ShipmentStatus)
		b.WriteString(fmt.Sprintf("🔔 <b>STATUS UPDATE</b>\n"))
		b.WriteString(separator + "\n")
		b.WriteString(fmt.Sprintf("📦 <code>%s</code>\n", code))
		b.WriteString(fmt.Sprintf("%s → <b>%s</b>\n", prevLabel, newLabel))
	} else if result.Delivered {
		b.WriteString("✅ <b>DELIVERED</b>\n")
		b.WriteString(separator + "\n")
		b.WriteString(fmt.Sprintf("📦 <code>%s</code>\n", code))
	} else {
		statusLabel := tracker.GetStatusLabel(result.ShipmentStatus)
		emoji := statusEmoji(result.ShipmentStatus)
		b.WriteString(fmt.Sprintf("%s <b>%s</b>\n", emoji, strings.ToUpper(statusLabel)))
		b.WriteString(separator + "\n")
		b.WriteString(fmt.Sprintf("📦 <code>%s</code>\n", code))
	}

	// Destination & weight (compact line)
	if result.Destination != "" {
		dest := cleanText(result.Destination)
		b.WriteString(fmt.Sprintf("📍 %s\n", dest))
	}
	if result.Weight > 0 {
		b.WriteString(fmt.Sprintf("⚖️  %.2f kg\n", result.Weight))
	}

	// Courier (only if out for delivery)
	if !result.Delivered && result.ShipmentStatus == "40" && result.CourierName != "" {
		b.WriteString(fmt.Sprintf("\n👤 <b>%s</b>\n📞 %s\n", result.CourierName, result.CourierMobile))
	}

	// Timeline
	if len(result.Events) > 0 {
		b.WriteString("\n📋 <b>Timeline</b>\n")
		start := len(result.Events) - 4
		if start < 0 {
			start = 0
		}
		// Show newest first
		for i := len(result.Events) - 1; i >= start; i-- {
			ev := result.Events[i]
			when := formatEventTime(ev.Time)
			emoji := eventEmoji(ev.Description)
			desc := cleanText(ev.Description)
			desc = shortenDescription(desc)
			b.WriteString(fmt.Sprintf("  %s <i>%s</i> — %s\n", emoji, when, desc))
		}
	}

	return b.String()
}

// statusEmoji returns the emoji for a shipment status code.
func statusEmoji(status string) string {
	switch status {
	case "10":
		return "📝"
	case "20":
		return "📋"
	case "30":
		return "🚚"
	case "40":
		return "🏃"
	case "50":
		return "⚠️"
	case "60":
		return "✅"
	case "70":
		return "❌"
	case "80":
		return "↩️"
	default:
		return "📦"
	}
}

// eventEmoji picks an emoji based on event description keywords.
func eventEmoji(desc string) string {
	d := strings.ToUpper(desc)
	switch {
	case strings.Contains(d, "SIGN") || strings.Contains(d, "DELIVERED"):
		return "✅"
	case strings.Contains(d, "DELIVER") && !strings.Contains(d, "ASSIGN"):
		return "🏃"
	case strings.Contains(d, "ASSIGN"):
		return "🚚"
	case strings.Contains(d, "ARRIV"):
		return "📦"
	case strings.Contains(d, "PICKUP") || strings.Contains(d, "PICKED"):
		return "📤"
	case strings.Contains(d, "TRANSIT"):
		return "🚛"
	case strings.Contains(d, "EXCEPTION") || strings.Contains(d, "FAIL"):
		return "⚠️"
	case strings.Contains(d, "RETURN"):
		return "↩️"
	default:
		return "📍"
	}
}

// formatEventTime converts "2026-04-29T20:36:20" to "Apr 29, 20:36"
func formatEventTime(raw string) string {
	if raw == "" {
		return ""
	}
	// Try ISO format first
	t, err := time.Parse("2006-01-02T15:04:05", strings.TrimSpace(raw[:min(len(raw), 19)]))
	if err != nil {
		// Fallback: just return as-is (cleaned)
		return strings.Replace(raw, "T", " ", 1)
	}
	return t.Format("Jan 2, 15:04")
}

// cleanText strips CJK brackets (replacing with space) and normalizes whitespace.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "【", " ")
	s = strings.ReplaceAll(s, "】", " ")
	s = strings.ReplaceAll(s, ",", ", ")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// shortenDescription keeps event text concise (max ~60 chars, single line).
func shortenDescription(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
