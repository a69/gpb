package msg

import "context"

// Messenger sends text messages to a chat or channel.
type Messenger interface {
	SendMessage(ctx context.Context, chatID, text string) error
}
