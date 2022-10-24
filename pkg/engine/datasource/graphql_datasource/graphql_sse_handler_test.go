package graphql_datasource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphQLSubscriptionClientSubscribe_SSE(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlQuery := r.URL.Query()
		assert.Equal(t, "subscription {messageAdded(roomName: \"room\"){text}}", urlQuery.Get("query"))

		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"messageAdded":{"text":"first"}}}`)
		flusher.Flush()

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"messageAdded":{"text":"second"}}}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next
	second := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_SSE_WithEvents(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "event: next\ndata: %s\n\n", `{"data":{"messageAdded":{"text":"first"}}}`)
		flusher.Flush()

		_, _ = fmt.Fprintf(w, "event: next\ndata: %s\n\n", `{"data":{"messageAdded":{"text":"second"}}}`)
		flusher.Flush()

		_, _ = fmt.Fprintf(w, "event: complete\n\n")
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next
	second := <-next

	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_SSE_Error(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"errors":[{"message":"Unexpected error.","locations":[{"line":2,"column":3}],"path":["countdown"]}]}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next

	assert.Equal(t, `{"errors":[{"message":"Unexpected error.","locations":[{"line":2,"column":3}],"path":["countdown"]}]}`, string(first))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_SSE_Error_Without_Header(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "%s\n\n", `{"errors":[{"message":"Unexpected error.","locations":[{"line":2,"column":3}],"path":["countdown"]}],"data":null}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next

	assert.Equal(t, `{"errors":[{"message":"Unexpected error.","locations":[{"line":2,"column":3}],"path":["countdown"]}]}`, string(first))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_QueryParams(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlQuery := r.URL.Query()
		assert.Equal(t, "subscription($a: Int!){countdown(from: $a)}", urlQuery.Get("query"))
		assert.Equal(t, "CountDown", urlQuery.Get("operationName"))
		assert.Equal(t, `{"a":5}`, urlQuery.Get("variables"))
		assert.Equal(t, `{"persistedQuery":{"version":1,"sha256Hash":"d41d8cd98f00b204e9800998ecf8427e"}}`, urlQuery.Get("extensions"))

		//Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"countdown":5}}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query:         `subscription($a: Int!){countdown(from: $a)}`,
			OperationName: "CountDown",
			Variables:     []byte(`{"a":5}`),
			Extensions:    []byte(`{"persistedQuery":{"version":1,"sha256Hash":"d41d8cd98f00b204e9800998ecf8427e"}}`),
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next
	assert.Equal(t, `{"data":{"countdown":5}}`, string(first))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}