package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	token  string
	apiURL string
}

func NewClient(token string) *Client {
	return &Client{
		token:  token,
		apiURL: "https://api.telegram.org/bot" + token,
	}
}

type SetWebhookReq struct {
	URL string `json:"url"`
}

type SendMessageReq struct {
	ChatID                interface{} `json:"chat_id"`
	Text                 string      `json:"text"`
	ParseMode            string      `json:"parse_mode,omitempty"`
	ReplyMarkup          interface{} `json:"reply_markup,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

func (c *Client) SetWebhook(webhookURL string) error {
	body := SetWebhookReq{URL: webhookURL}
	return c.post("setWebhook", body)
}

func (c *Client) DeleteWebhook() error {
	return c.post("deleteWebhook", struct{}{})
}

func (c *Client) SendMessage(chatID interface{}, text, parseMode string) error {
	body := SendMessageReq{
		ChatID:    chatID,
		Text:      text,
		ParseMode: parseMode,
	}
	return c.post("sendMessage", body)
}

func (c *Client) SendMessageWithKeyboard(chatID interface{}, text, parseMode string, keyboard [][]InlineKeyboardButton) error {
	body := SendMessageReq{
		ChatID:    chatID,
		Text:      text,
		ParseMode: parseMode,
		ReplyMarkup: InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	}
	return c.post("sendMessage", body)
}

func (c *Client) post(method string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		c.apiURL+"/"+method,
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error: %d %s", resp.StatusCode, string(body))
	}

	return nil
}
