package resolve

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type startedTrigger struct {
	input   string
	updater SubscriptionUpdater
}

// collidingSubscriptionSource is shaped like the real graphql datasource: its UniqueRequestID
// identifies the upstream *connection* (URL, headers, ...) and never the operation, so every
// subscription it serves gets the same trigger ID no matter which operation it carries.
type collidingSubscriptionSource struct {
	mu       sync.Mutex
	started  []startedTrigger
	messages map[string]string // rendered input -> message the upstream pushes for that operation
}

func (s *collidingSubscriptionSource) UniqueRequestID(_ *Context, _ []byte, xxh *xxhash.Digest) error {
	_, err := xxh.WriteString("same-upstream-connection")
	return err
}

func (s *collidingSubscriptionSource) Start(_ *Context, input []byte, updater SubscriptionUpdater) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = append(s.started, startedTrigger{input: string(input), updater: updater})
	return nil
}

// pushUpdates makes every started trigger push the message configured for its operation. It is
// called from the test rather than from Start, because Start itself runs on the resolver's event
// loop - the very goroutine that has to receive the update.
func (s *collidingSubscriptionSource) pushUpdates() {
	s.mu.Lock()
	started := append([]startedTrigger(nil), s.started...)
	messages := s.messages
	s.mu.Unlock()

	for _, trigger := range started {
		trigger.updater.Update([]byte(messages[trigger.input]))
	}
}

func (s *collidingSubscriptionSource) startedInputs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	inputs := make([]string, 0, len(s.started))
	for _, trigger := range s.started {
		inputs = append(inputs, trigger.input)
	}
	return inputs
}

// uncomparableSubscriptionSource is a data source whose dynamic type cannot be compared with ==,
// which would panic if sameSubscriptionSource compared it anyway.
type uncomparableSubscriptionSource []string

func (uncomparableSubscriptionSource) UniqueRequestID(_ *Context, _ []byte, _ *xxhash.Digest) error {
	return nil
}

func (uncomparableSubscriptionSource) Start(_ *Context, _ []byte, _ SubscriptionUpdater) error {
	return nil
}

// TestSameSubscriptionSource covers the check that decides whether a subscription may join an
// existing trigger. Answering "yes" for two sources that are not actually the same upstream is
// what feeds one subscriber another one's data, so everything but a genuine identity is a "no".
func TestSameSubscriptionSource(t *testing.T) {
	source := &collidingSubscriptionSource{}
	sameTypeOtherInstance := &collidingSubscriptionSource{}
	otherType := createFakeStream(func(counter int) (message string, done bool) {
		return "", true
	}, 0, nil)

	t.Run("the same instance is the same source", func(t *testing.T) {
		assert.True(t, sameSubscriptionSource(source, source))
	})

	t.Run("two instances of the same type are two sources", func(t *testing.T) {
		assert.False(t, sameSubscriptionSource(source, sameTypeOtherInstance))
	})

	t.Run("different types are never the same source", func(t *testing.T) {
		assert.False(t, sameSubscriptionSource(source, otherType))
		assert.False(t, sameSubscriptionSource(otherType, source))
	})

	t.Run("nil is only the same as nil", func(t *testing.T) {
		assert.True(t, sameSubscriptionSource(nil, nil))
		assert.False(t, sameSubscriptionSource(source, nil))
		assert.False(t, sameSubscriptionSource(nil, source))
	})

	t.Run("an uncomparable source is rejected instead of panicking", func(t *testing.T) {
		// Comparing interfaces that hold an uncomparable type panics at runtime, which would take
		// down the resolver's event loop rather than just fail to reuse a trigger.
		assert.NotPanics(t, func() {
			assert.False(t, sameSubscriptionSource(
				uncomparableSubscriptionSource{"a"},
				uncomparableSubscriptionSource{"a"},
			))
		})
	})
}

func collidingSubscriptionPlan(source SubscriptionDataSource, input string) *GraphQLSubscription {
	return &GraphQLSubscription{
		Trigger: GraphQLSubscriptionTrigger{
			Source: source,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(input),
					},
				},
			},
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath:   []string{"data"},
				SelectResponseErrorsPath: []string{"errors"},
			},
		},
		Response: &GraphQLResponse{
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("counter"),
						Value: &Integer{
							Path: []string{"counter"},
						},
					},
				},
			},
		},
	}
}

// TestResolver_SubscriptionsAreNotMergedByTriggerIDAlone guards against unrelated subscriptions
// being merged onto one trigger. The trigger ID only identifies the upstream connection, so two
// subscriptions can share it while asking for entirely different operations - merging them means
// the second operation is never started upstream and its subscriber is fed the first one's data.
func TestResolver_SubscriptionsAreNotMergedByTriggerIDAlone(t *testing.T) {
	const (
		firstInput  = `{"url":"http://localhost:4000","body":{"query":"subscription { first }"}}`
		secondInput = `{"url":"http://localhost:4000","body":{"query":"subscription { second }"}}`

		firstMessage  = `{"data":{"counter":1}}`
		secondMessage = `{"data":{"counter":2}}`
	)

	subscribe := func(t *testing.T, resolver *Resolver, plan *GraphQLSubscription, id int64) *SubscriptionRecorder {
		t.Helper()

		recorder := &SubscriptionRecorder{
			buf:      &bytes.Buffer{},
			messages: []string{},
		}
		require.NoError(t, resolver.AsyncResolveGraphQLSubscription(
			NewContext(context.Background()),
			plan,
			recorder,
			SubscriptionIdentifier{ConnectionID: id, SubscriptionID: id},
		))
		return recorder
	}

	// The resolver handles one event at a time, so once it has picked up an event, every event
	// queued before it - every subscription added above - has already been handled completely.
	// Removing a client that does not exist is a no-op that travels through the same channel.
	syncWithEventLoop := func(t *testing.T, resolver *Resolver) {
		t.Helper()
		require.NoError(t, resolver.AsyncUnsubscribeClient(-1))
	}

	t.Run("different operations over the same connection", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		resolver := newResolver(ctx)

		source := &collidingSubscriptionSource{messages: map[string]string{
			firstInput:  firstMessage,
			secondInput: secondMessage,
		}}

		first := subscribe(t, resolver, collidingSubscriptionPlan(source, firstInput), 1)
		second := subscribe(t, resolver, collidingSubscriptionPlan(source, secondInput), 2)
		syncWithEventLoop(t, resolver)

		assert.ElementsMatch(t, []string{firstInput, secondInput}, source.startedInputs(),
			"both operations must be started upstream, not just the one that created the trigger")

		source.pushUpdates()
		first.AwaitMessages(t, 1, time.Second)
		second.AwaitMessages(t, 1, time.Second)

		assert.Equal(t, []string{firstMessage}, first.Messages())
		assert.Equal(t, []string{secondMessage}, second.Messages(),
			"a subscription must only ever see the data of its own operation")
	})

	t.Run("different sources with the same input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		resolver := newResolver(ctx)

		firstSource := &collidingSubscriptionSource{messages: map[string]string{firstInput: firstMessage}}
		secondSource := &collidingSubscriptionSource{messages: map[string]string{firstInput: secondMessage}}

		first := subscribe(t, resolver, collidingSubscriptionPlan(firstSource, firstInput), 1)
		second := subscribe(t, resolver, collidingSubscriptionPlan(secondSource, firstInput), 2)
		syncWithEventLoop(t, resolver)

		assert.Equal(t, []string{firstInput}, firstSource.startedInputs())
		assert.Equal(t, []string{firstInput}, secondSource.startedInputs(),
			"a subscription served by a different data source must not be attached to another source's trigger")

		firstSource.pushUpdates()
		secondSource.pushUpdates()
		first.AwaitMessages(t, 1, time.Second)
		second.AwaitMessages(t, 1, time.Second)

		assert.Equal(t, []string{firstMessage}, first.Messages())
		assert.Equal(t, []string{secondMessage}, second.Messages())
	})

	// Regression guard: splitting triggers must not go so far that identical subscriptions stop
	// sharing one upstream connection.
	t.Run("identical subscriptions still share one trigger", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		resolver := newResolver(ctx)

		source := &collidingSubscriptionSource{messages: map[string]string{firstInput: firstMessage}}

		first := subscribe(t, resolver, collidingSubscriptionPlan(source, firstInput), 1)
		second := subscribe(t, resolver, collidingSubscriptionPlan(source, firstInput), 2)
		syncWithEventLoop(t, resolver)

		assert.Equal(t, []string{firstInput}, source.startedInputs(),
			"identical subscriptions must be multiplexed onto a single upstream operation")

		source.pushUpdates()
		first.AwaitMessages(t, 1, time.Second)
		second.AwaitMessages(t, 1, time.Second)

		assert.Equal(t, []string{firstMessage}, first.Messages())
		assert.Equal(t, []string{firstMessage}, second.Messages())
	})
}
