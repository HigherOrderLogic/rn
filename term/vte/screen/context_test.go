package screen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsScreenContext(t *testing.T) {
	t.Run("returns false if context is not screen", func(t *testing.T) {
		assert.False(t, IsScreenContext(context.Background()))
	})
	t.Run("returns true if context is screen", func(t *testing.T) {
		ctx := NewContext(context.Background())
		assert.True(t, IsScreenContext(ctx))
	})
	t.Run("returns true if context double set screen", func(t *testing.T) {
		ctx := NewContext(NewContext(context.Background()))
		assert.True(t, IsScreenContext(ctx))
	})
	t.Run("returns true if is derivative", func(t *testing.T) {
		ctx, cancel := context.WithCancel(NewContext(context.Background()))
		cancel()
		assert.True(t, IsScreenContext(ctx))
	})
}
