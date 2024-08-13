package mailstream

import "context"

type ContextKey string

const MAILSTREAM_KEY ContextKey = "mailstream"

func WithContext(ctx context.Context, client *Client) context.Context {
	return context.WithValue(ctx, MAILSTREAM_KEY, client)
}

func FromContext(ctx context.Context) *Client {
	client, ok := ctx.Value(MAILSTREAM_KEY).(*Client)
	if !ok {
		return nil
	}
	return client
}
