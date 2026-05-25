package ratelimit

import (
	"fmt"
	"time"

	"github.com/dashsyy/ce-tracker/internal/state"
)

func CanAddCode(appState *state.AppState, chatID string, maxCodesPerUser int) (bool, string) {
	chat := appState.Chats[chatID]
	if len(chat.Codes) >= maxCodesPerUser {
		return false, fmt.Sprintf("❌ Limit reached. You can track up to %d codes. Delete old ones or check existing ones.", maxCodesPerUser)
	}
	return true, ""
}

func CanCheckCode(appState *state.AppState, chatID, code string, minCheckInterval int) (bool, string) {
	chat := appState.Chats[chatID]
	if prevState, exists := chat.CodeStates[code]; exists {
		checkedTime, err := time.Parse(time.RFC3339, prevState.CheckedAt)
		if err == nil {
			elapsed := time.Since(checkedTime).Seconds()
			if elapsed < float64(minCheckInterval) {
				minsLeft := int((float64(minCheckInterval)-elapsed)/60) + 1
				return false, fmt.Sprintf("⏳ <code>%s</code> was checked recently. Try again in %d minute(s).", code, minsLeft)
			}
		}
	}
	return true, ""
}
