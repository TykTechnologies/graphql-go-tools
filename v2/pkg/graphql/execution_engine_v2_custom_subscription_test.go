package graphql

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

// TestCustomExecutionEngineV2Executor_AsyncSubscription_NoUseAfterFree exercises the
// async subscription path of the custom executor. The executor used to free the
// resolve.Context in a deferred call inside Execute, while AsyncResolveGraphQLSubscription
// had already enqueued that same *resolve.Context onto the resolver's event loop.
// The async loop later dereferenced the freed context and crashed inside
// xcontext.Detach -> context.WithCancel.
//
// The repro:
//   - Build a CustomExecutionEngineV2Executor.
//   - Resolver stage returns a SubscriptionResponsePlan and resolves through the real
//     resolver via AsyncResolveGraphQLSubscription, just like ExecutionEngineV2 does.
//   - A fake stream emits 3 events then completes.
//   - Without the fix this test panics inside the resolver goroutine; with the fix
//     all three events arrive in order and the writer sees a Complete().
func TestCustomExecutionEngineV2Executor_AsyncSubscription_NoUseAfterFree(t *testing.T) {
	defaultTimeout := 10 * time.Second

	rCtx, rCancel := context.WithCancel(context.Background())
	defer rCancel()

	resolver := resolve.New(rCtx, resolve.ResolverOptions{
		MaxConcurrency: 16,
	})

	// Simple subscription plan with a fake stream that emits {"data":{"counter":N}} until N==2.
	stream := newFakeSubscriptionStream(func(counter int) (string, bool) {
		return fmt.Sprintf(`{"data":{"counter":%d}}`, counter), counter == 2
	})

	subscriptionPlan := &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Source: stream,
				InputTemplate: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							SegmentType: resolve.StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`),
						},
					},
				},
				PostProcessing: resolve.PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("counter"),
							Value: &resolve.Integer{
								Path: []string{"counter"},
							},
						},
					},
				},
			},
		},
	}

	stage := &subscriptionResolverStage{
		resolver: resolver,
		plan:     subscriptionPlan,
	}

	executor, err := NewCustomExecutionEngineV2ExecutorByStages(CustomExecutionEngineV2Stages{
		RequiredStages: CustomExecutionEngineV2RequiredStages{
			ResolverStage: stage,
		},
	})
	require.NoError(t, err)

	recorder := newSubscriptionRecorder()

	// The Tyk gateway calls Execute with a per-request context. Use a context with
	// a cancel func so the test cleans up if anything stalls.
	execCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run Execute on a goroutine and detect panics.
	done := make(chan struct{})
	var panicValue interface{}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicValue = r
			}
			close(done)
		}()
		execErr := executor.Execute(execCtx, &Request{}, recorder)
		assert.NoError(t, execErr)
	}()

	select {
	case <-done:
	case <-time.After(defaultTimeout):
		t.Fatal("Execute did not return within timeout")
	}
	require.Nil(t, panicValue, "Execute panicked: %v", panicValue)

	// The subscription writes happen on the resolver's goroutine. The resolver loop
	// may panic *after* Execute returns when it tries to call xcontext.Detach on
	// the freed context. Wait for all 3 messages and Complete; if either fails to
	// arrive within the timeout the bug is reproduced.
	recorder.AwaitMessageCount(t, 3, defaultTimeout)
	recorder.AwaitComplete(t, defaultTimeout)

	messages := recorder.Messages()
	assert.ElementsMatch(t, []string{
		`{"data":{"counter":0}}`,
		`{"data":{"counter":1}}`,
		`{"data":{"counter":2}}`,
	}, messages)
}

// subscriptionResolverStage mimics ExecutionEngineV2.Resolve for a subscription plan.
// It deliberately uses AsyncResolveGraphQLSubscription so the test exercises the
// exact use-after-free path the bug lives on.
type subscriptionResolverStage struct {
	resolver *resolve.Resolver
	plan     *plan.SubscriptionResponsePlan
}

func (s *subscriptionResolverStage) Setup(ctx context.Context, postProcessor *postprocess.Processor, resolveContext *resolve.Context, operation *Request, options ...ExecutionOptionsV2) {
}

func (s *subscriptionResolverStage) Plan(postProcessor *postprocess.Processor, operation *Request, report *operationreport.Report) (plan.Plan, error) {
	return s.plan, nil
}

func (s *subscriptionResolverStage) Resolve(resolveContext *resolve.Context, planResult plan.Plan, writer resolve.SubscriptionResponseWriter) error {
	subPlan := planResult.(*plan.SubscriptionResponsePlan)
	return s.resolver.AsyncResolveGraphQLSubscription(resolveContext, subPlan.Response, writer, resolve.SubscriptionIdentifier{})
}

func (s *subscriptionResolverStage) Teardown() {}

func (s *subscriptionResolverStage) Normalize(operation *Request) error         { return nil }
func (s *subscriptionResolverStage) ValidateForSchema(operation *Request) error { return nil }
func (s *subscriptionResolverStage) InputValidation(operation *Request) error   { return nil }

// fakeSubscriptionStream is a minimal SubscriptionDataSource that emits messages
// from a callback until the callback signals done.
type fakeSubscriptionStream struct {
	messageFunc func(counter int) (string, bool)
	isDone      atomic.Bool
}

func newFakeSubscriptionStream(messageFunc func(counter int) (string, bool)) *fakeSubscriptionStream {
	return &fakeSubscriptionStream{messageFunc: messageFunc}
}

func (f *fakeSubscriptionStream) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) error {
	if _, err := xxh.WriteString("fakeSubscriptionStream"); err != nil {
		return err
	}
	_, err := xxh.Write(input)
	return err
}

func (f *fakeSubscriptionStream) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	go func() {
		counter := 0
		for {
			select {
			case <-ctx.Context().Done():
				updater.Done()
				f.isDone.Store(true)
				return
			default:
			}
			message, done := f.messageFunc(counter)
			updater.Update([]byte(message))
			if done {
				updater.Done()
				f.isDone.Store(true)
				return
			}
			counter++
		}
	}()
	return nil
}

// subscriptionRecorder is a SubscriptionResponseWriter that captures each Flush()
// as a complete message, and signals Complete().
type subscriptionRecorder struct {
	mu       sync.Mutex
	buf      *bytes.Buffer
	messages []string
	complete atomic.Bool
}

func newSubscriptionRecorder() *subscriptionRecorder {
	return &subscriptionRecorder{
		buf: &bytes.Buffer{},
	}
}

func (s *subscriptionRecorder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *subscriptionRecorder) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, s.buf.String())
	s.buf.Reset()
}

func (s *subscriptionRecorder) Complete() {
	s.complete.Store(true)
}

func (s *subscriptionRecorder) Messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.messages))
	copy(out, s.messages)
	return out
}

func (s *subscriptionRecorder) AwaitMessageCount(t *testing.T, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		current := len(s.messages)
		s.mu.Unlock()
		if current >= count {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d messages, got: %v", count, s.Messages())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *subscriptionRecorder) AwaitComplete(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if s.complete.Load() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for Complete()")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
