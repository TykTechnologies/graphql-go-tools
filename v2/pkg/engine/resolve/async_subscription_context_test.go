package resolve

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type asyncSubscriptionContextKey struct{}

// resolverWithoutEventLoop builds a Resolver that does not run handleEvents, so the test itself
// can receive the subscription event exactly where the event loop would. New() always starts the
// event loop and r.events is unbuffered, which would make "who owns the Context?" a race.
func resolverWithoutEventLoop() *Resolver {
	return &Resolver{
		ctx:      context.Background(),
		events:   make(chan subscriptionEvent),
		triggers: make(map[uint64]*trigger),
	}
}

func asyncSubscriptionPlan() *GraphQLSubscription {
	return &GraphQLSubscription{
		Trigger: GraphQLSubscriptionTrigger{
			Source: createFakeStream(func(counter int) (message string, done bool) {
				return "", true
			}, 0, nil),
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`),
					},
				},
			},
		},
		Response: &GraphQLResponse{
			Data: &Object{},
		},
	}
}

// captureAsyncSubscriptionContext starts an async subscription and returns the *Context the
// resolver handed over to its event loop. Only the pointer is captured here - the values behind
// it are read by the caller afterwards, which is what makes "did the resolver keep a reference to
// our Context?" observable without any timing assumptions.
func captureAsyncSubscriptionContext(t *testing.T, resolver *Resolver, ctx *Context) *Context {
	t.Helper()

	captured := make(chan *Context, 1)
	go func() {
		event := <-resolver.events
		captured <- event.addSubscription.ctx
	}()

	recorder := &SubscriptionRecorder{
		buf:      &bytes.Buffer{},
		messages: []string{},
	}
	id := SubscriptionIdentifier{ConnectionID: 1, SubscriptionID: 1}

	err := resolver.AsyncResolveGraphQLSubscription(ctx, asyncSubscriptionPlan(), recorder, id)
	require.NoError(t, err)

	select {
	case handedOver := <-captured:
		return handedOver
	case <-time.After(time.Second):
		t.Fatal("resolver did not hand the subscription over to its event loop")
		return nil
	}
}

// TestAsyncResolveGraphQLSubscriptionOwnsCallerContext guards against the resolver keeping the
// caller's *Context alive past the call. AsyncResolveGraphQLSubscription returns immediately, but
// the event loop only dereferences the Context later, in handleAddSubscription - by which time the
// caller (internalExecutionContext.reset() -> Free()) has long returned it to its pool and
// prepared it for the next request. The subscription must be started with the request it belongs
// to, not with whatever request happens to own the pooled Context by then.
func TestAsyncResolveGraphQLSubscriptionOwnsCallerContext(t *testing.T) {
	resolver := resolverWithoutEventLoop()

	reqCtx := NewContext(context.Background())
	reqCtx.Variables = []byte(`{"room":"first"}`)
	reqCtx.Request.Header = http.Header{"X-Request-Id": {"first"}}
	reqCtx.InitialPayload = []byte(`{"token":"first"}`)
	reqCtx.Extensions = []byte(`{"trace":"first"}`)

	handedOver := captureAsyncSubscriptionContext(t, resolver, reqCtx)
	require.NotNil(t, handedOver)

	// The caller returns its pooled Context and prepares it for the next request.
	reqCtx.Free()
	reqCtx.Variables = []byte(`{"room":"second"}`)
	reqCtx.Request.Header = http.Header{"X-Request-Id": {"second"}}
	reqCtx.InitialPayload = []byte(`{"token":"second"}`)
	reqCtx.Extensions = []byte(`{"trace":"second"}`)

	assert.Equal(t, `{"room":"first"}`, string(handedOver.Variables),
		"the subscription must keep its own request's variables, not the ones of the request that reused the pooled Context")
	assert.Equal(t, "first", handedOver.Request.Header.Get("X-Request-Id"),
		"the subscription must keep its own request's headers, not the ones of the request that reused the pooled Context")
	assert.Equal(t, `{"token":"first"}`, string(handedOver.InitialPayload),
		"the subscription must keep its own connection init payload, not the one of the request that reused the pooled Context")
	assert.Equal(t, `{"trace":"first"}`, string(handedOver.Extensions),
		"the subscription must keep its own request's extensions, not the ones of the request that reused the pooled Context")
}

// TestAsyncResolveGraphQLSubscriptionKeepsRequestContextAfterCallerFree guards against the
// subscription losing its request context.Context when the caller frees the pooled Context.
// Free() nils it, and handleAddSubscription later builds the trigger's context from it -
// a nil parent there detaches the subscription from the request it was started for.
func TestAsyncResolveGraphQLSubscriptionKeepsRequestContextAfterCallerFree(t *testing.T) {
	resolver := resolverWithoutEventLoop()

	requestContext := context.WithValue(context.Background(), asyncSubscriptionContextKey{}, "first")
	reqCtx := NewContext(requestContext)

	handedOver := captureAsyncSubscriptionContext(t, resolver, reqCtx)
	require.NotNil(t, handedOver)

	reqCtx.Free()

	require.NotNil(t, handedOver.Context(), "the subscription must keep the request context it was started with")
	assert.Equal(t, "first", handedOver.Context().Value(asyncSubscriptionContextKey{}))
}

// TestContextCloneCopiesInitialPayloadAndExtensions guards against clone() sharing the original's
// InitialPayload/Extensions buffers - a clone handed to a long running subscription would then
// change underneath it whenever the original is reused.
func TestContextCloneCopiesInitialPayloadAndExtensions(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.InitialPayload = []byte(`{"token":"current"}`)
	ctx.Extensions = []byte(`{"trace":"current"}`)

	clone := ctx.clone(context.Background())

	// Overwritten in place: a clone that shares the original's backing array changes with it.
	copy(ctx.InitialPayload, []byte(`{"token":"changed"}`))
	copy(ctx.Extensions, []byte(`{"trace":"changed"}`))

	assert.Equal(t, `{"token":"current"}`, string(clone.InitialPayload), "clone() must copy InitialPayload, not share it")
	assert.Equal(t, `{"trace":"current"}`, string(clone.Extensions), "clone() must copy Extensions, not share it")
}

// TestContextFreeClearsInitialPayload guards against a pooled Context leaking one request's
// connection init payload into the next request that reuses it.
func TestContextFreeClearsInitialPayload(t *testing.T) {
	ctx := NewContext(context.Background())
	ctx.InitialPayload = []byte(`{"token":"stale"}`)

	ctx.Free()

	assert.Nil(t, ctx.InitialPayload, "Context.Free() must clear InitialPayload so a reused Context doesn't leak it into an unrelated later request")
}
