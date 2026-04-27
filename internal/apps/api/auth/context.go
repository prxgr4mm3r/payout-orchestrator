package auth

import "context"

type Client struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type clientContextKey struct{}

func WithClient(ctx context.Context, client Client) context.Context {
	return context.WithValue(ctx, clientContextKey{}, client)
}

func ClientFromContext(ctx context.Context) (Client, bool) {
	client, ok := ctx.Value(clientContextKey{}).(Client)
	return client, ok
}
