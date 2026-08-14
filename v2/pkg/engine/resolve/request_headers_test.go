package resolve

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContextFreeClearsRequestHeaderOptions guards against a pooled Context
// leaking one request's UpstreamHeaders/HeaderModifier into the next request
// that reuses it after Free().
func TestContextFreeClearsRequestHeaderOptions(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.UpstreamHeaders = http.Header{"Authorization": {"Bearer stale"}}
	ctx.HeaderModifier = func(http.Header) {}

	ctx.Free()

	assert.Nil(t, ctx.UpstreamHeaders, "Context.Free() must clear UpstreamHeaders so a reused Context doesn't leak it into an unrelated later request")
	assert.Nil(t, ctx.HeaderModifier, "Context.Free() must clear HeaderModifier so a reused Context doesn't leak it into an unrelated later request")
}

// TestContextCloneIsolatesRequestHeaderOptions guards against clone() sharing
// the same underlying UpstreamHeaders map as the original - that would let
// unrelated concurrent work on the original Context (or the original's later
// reuse) affect a clone that should be independent.
func TestContextCloneIsolatesRequestHeaderOptions(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.UpstreamHeaders = http.Header{"Authorization": {"Bearer current"}}
	ctx.HeaderModifier = func(header http.Header) { header.Set("X-Caller", "current") }

	clone := ctx.clone(context.Background())

	require.NotNil(t, clone.HeaderModifier, "clone() must preserve HeaderModifier, not drop it")
	require.NotNil(t, clone.UpstreamHeaders, "clone() must preserve UpstreamHeaders, not drop it")

	// Mutating the original's map after cloning must not affect the clone -
	// otherwise a cloned Context used for independent field resolution could
	// be changed by unrelated work still using the original.
	ctx.UpstreamHeaders.Set("Authorization", "Bearer changed")

	assert.Equal(t, "Bearer current", clone.UpstreamHeaders.Get("Authorization"))
}
