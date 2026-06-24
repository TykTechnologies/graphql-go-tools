package subscription

import (
	"context"
	"net/http"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
	"github.com/TykTechnologies/graphql-go-tools/pkg/postprocess"
)

// stubFlushWriterEngine captures the context the executor hands to the engine,
// so we can assert what does (and does not) propagate to upstream fetches.
type stubFlushWriterEngine struct {
	gotCtx context.Context
}

func (s *stubFlushWriterEngine) Execute(ctx context.Context, _ *graphql.Request, _ resolve.FlushWriter, _ ...graphql.ExecutionOptionsV2) error {
	s.gotCtx = ctx
	return nil
}

func (s *stubFlushWriterEngine) GetWebsocketBeforeStartHook() graphql.WebsocketBeforeStartHook {
	return nil
}

// TestExecutorV2DoesNotLeakRequestContextValuesToEngine guards against the
// regression where the original websocket-upgrade request context leaked into
// the engine execution context. The host application (e.g. Tyk) stores
// request-scoped flags such as "this request is a websocket upgrade" as context
// values on the initial request. Those values must NOT reach the engine that
// fetches data from upstream/subgraphs, otherwise plain queries run over a
// websocket connection are mistaken for websocket upgrades and return null.
func TestExecutorV2DoesNotLeakRequestContextValuesToEngine(t *testing.T) {
	type ctxKey string
	const upgradeKey ctxKey = "graphqlIsWebSocketUpgrade"

	// reqCtx simulates the original websocket request context carrying a flag.
	reqCtx := context.WithValue(context.Background(), upgradeKey, true)

	engine := &stubFlushWriterEngine{}
	executor := &ExecutorV2{
		engine:         engine,
		operation:      &graphql.Request{},
		context:        context.Background(),
		reqCtx:         reqCtx,
		headerModifier: func(http.Header) {},
	}

	if err := executor.Execute(nil); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if engine.gotCtx == nil {
		t.Fatal("engine was not invoked")
	}
	if got := engine.gotCtx.Value(upgradeKey); got != nil {
		t.Fatalf("request context value %q leaked into engine context: got %v, want nil", upgradeKey, got)
	}
	// The header modifier must still be propagated (preserves TT-17442's
	// per-user connection/credential isolation).
	if postprocess.GetHeaderModifier(engine.gotCtx) == nil {
		t.Fatal("header modifier was not propagated to the engine context")
	}
}

// TestExecutorV2PropagatesSubscriptionContextCancellation ensures the engine
// context's lifetime is the per-subscription context (e.context), so cancelling
// the subscription still cancels execution.
func TestExecutorV2PropagatesSubscriptionContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	engine := &stubFlushWriterEngine{}
	executor := &ExecutorV2{
		engine:    engine,
		operation: &graphql.Request{},
		context:   ctx,
		reqCtx:    context.Background(),
	}

	if err := executor.Execute(nil); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	cancel()

	if engine.gotCtx.Err() != context.Canceled {
		t.Fatalf("expected engine context to be canceled, got %v", engine.gotCtx.Err())
	}
}
