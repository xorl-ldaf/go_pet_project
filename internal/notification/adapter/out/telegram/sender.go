package telegram

import (
	"context"
	"strings"

	notificationout "go_pet_project/internal/notification/application/port/out"
)

var _ notificationout.MessageSender = (*Sender)(nil)

type Sender struct {
	client *Client
}

func NewSender(client *Client) *Sender {
	return &Sender{client: client}
}

func (s *Sender) Send(ctx context.Context, message notificationout.Message) error {
	text := strings.TrimSpace(message.Title)
	if body := strings.TrimSpace(message.Body); body != "" {
		if text != "" {
			text += "\n\n"
		}
		text += body
	}

	return s.client.SendText(ctx, message.ChatID, text)
}
