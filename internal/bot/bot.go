package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Client struct {
	token  string
	httpc  *http.Client
	apiURL string
}

func NewClient() *Client {
	tok := os.Getenv("TG_BOT_TOKEN")
	return &Client{
		token:  tok,
		apiURL: "https://api.telegram.org/bot" + tok,
		httpc:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) send(method string, payload any) error {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", c.apiURL+"/"+method, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram %s: %s", method, resp.Status)
	}
	return nil
}

func (c *Client) SendMessage(chatID int64, text string, replyMarkup any) error {
	data := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if replyMarkup != nil {
		data["reply_markup"] = replyMarkup
	}
	return c.send("sendMessage", data)
}

func (c *Client) SendPhoto(chatID int64, photoURL, caption string, replyMarkup any) error {
	data := map[string]any{
		"chat_id": chatID,
		"photo":   photoURL, // we’ll use your /qr/{code}.png
	}
	if caption != "" {
		data["caption"] = caption
	}
	if replyMarkup != nil {
		data["reply_markup"] = replyMarkup
	}
	return c.send("sendPhoto", data)
}
