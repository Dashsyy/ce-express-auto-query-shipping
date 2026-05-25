package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CodeState struct {
	ShipmentStatus string `json:"shipmentStatus"`
	LastEventCode  string `json:"lastEventCode"`
	LastEventTime  string `json:"lastEventTime"`
	Delivered      bool   `json:"delivered"`
	CheckedAt      string `json:"checkedAt"`
}

type ChatState struct {
	Codes      []string              `json:"codes"`
	CodeStates map[string]CodeState  `json:"code_states"`
}

type AppState struct {
	Chats map[string]ChatState `json:"chats"`
}

type Manager struct {
	filePath string
	mu       sync.RWMutex
}

func NewManager(filePath string) *Manager {
	return &Manager{
		filePath: filePath,
	}
}

func (m *Manager) Load() (*AppState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &AppState{Chats: make(map[string]ChatState)}, nil
		}
		return nil, err
	}

	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.Chats == nil {
		state.Chats = make(map[string]ChatState)
	}

	return &state, nil
}

func (m *Manager) Save(state *AppState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(m.filePath), 0755); err != nil && !os.IsExist(err) {
		return err
	}

	return os.WriteFile(m.filePath, data, 0644)
}

func (m *Manager) GetChat(state *AppState, chatID string) ChatState {
	if chat, exists := state.Chats[chatID]; exists {
		return chat
	}
	return ChatState{
		Codes:      []string{},
		CodeStates: make(map[string]CodeState),
	}
}

func (m *Manager) SetChat(state *AppState, chatID string, chat ChatState) {
	state.Chats[chatID] = chat
}

func (m *Manager) UpdateCodeState(state *AppState, chatID, code string, codeState CodeState) {
	if chat, exists := state.Chats[chatID]; exists {
		if chat.CodeStates == nil {
			chat.CodeStates = make(map[string]CodeState)
		}
		codeState.CheckedAt = time.Now().UTC().Format(time.RFC3339)
		chat.CodeStates[code] = codeState
		state.Chats[chatID] = chat
	}
}
