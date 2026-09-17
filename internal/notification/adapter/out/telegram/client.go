package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.telegram.org"

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

func NewClient(token string) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}

	return &Client{
		token:   token,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 35 * time.Second,
		},
	}, nil
}

func (c *Client) SendText(ctx context.Context, chatID int64, text string) error {
	values := url.Values{}
	values.Set("chat_id", strconv.FormatInt(chatID, 10))
	values.Set("text", text)

	var response telegramResponse
	if err := c.do(ctx, "sendMessage", values, &response); err != nil {
		return err
	}
	if !response.OK {
		return fmt.Errorf("telegram sendMessage failed: %s", response.Description)
	}

	return nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int, timeout time.Duration) ([]Update, error) {
	values := url.Values{}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	values.Set("timeout", strconv.Itoa(int(timeout.Seconds())))

	var response updatesResponse
	if err := c.do(ctx, "getUpdates", values, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("telegram getUpdates failed: %s", response.Description)
	}

	return response.Result, nil
}

func (c *Client) do(ctx context.Context, method string, values url.Values, target interface{}) error {
	endpoint := fmt.Sprintf("%s/bot%s/%s", strings.TrimRight(c.baseURL, "/"), c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s request failed", method)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode telegram %s response: %w", method, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram %s failed with status %d", method, resp.StatusCode)
	}

	return nil
}

type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

type updatesResponse struct {
	OK          bool     `json:"ok"`
	Description string   `json:"description"`
	Result      []Update `json:"result"`
}

type Update struct {
	UpdateID int      `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	Chat *Chat  `json:"chat"`
	From *User  `json:"from"`
	Text string `json:"text"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type User struct {
	Username string `json:"username"`
}
