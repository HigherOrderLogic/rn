package screen

import (
	"context"
)

type ctxKey int

var screenIDKey ctxKey

// NewContext returns a new Context that holds a screen context key,
// such that any contexts that derive from it, including itself, would
// return true in when passed on call to IsScreenContext.
func NewContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, screenIDKey, struct{}{})
}

// IsScreenContext returns the ID value stored in ctx, if any.
func IsScreenContext(ctx context.Context) bool {
	v := ctx.Value(screenIDKey)
	return v != nil
}
