package out

import "context"

type Message struct {
	ChatID int64
	Title  string
	Body   string
}

type MessageSender interface {
	Send(ctx context.Context, message Message) error
}
