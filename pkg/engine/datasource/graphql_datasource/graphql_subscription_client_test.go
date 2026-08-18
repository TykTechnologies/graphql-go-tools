package graphql_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	nhooyrwebsocket "github.com/coder/websocket"
	ll "github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/TykTechnologies/graphql-go-tools/pkg/postprocess"
)

func logger() ll.Logger {
	logger, err := zap.NewDevelopmentConfig().Build()
	if err != nil {
		panic(err)
	}

	return ll.NewZapLogger(logger, ll.DebugLevel)
}

func TestGetConnectionInitMessageHelper(t *testing.T) {
	var callback OnWsConnectionInitCallback = func(ctx context.Context, url string, header http.Header) (json.RawMessage, error) {
		return json.RawMessage(`{"authorization":"secret"}`), nil
	}

	tests := []struct {
		name     string
		callback *OnWsConnectionInitCallback
		want     string
	}{
		{
			name:     "without payload",
			callback: nil,
			want:     `{"type":"connection_init"}`,
		},
		{
			name:     "with payload",
			callback: &callback,
			want:     `{"type":"connection_init","payload":{"authorization":"secret"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := SubscriptionClient{onWsConnectionInitCallback: tt.callback}
			got, err := client.getConnectionInitMessage(context.Background(), "", nil)
			require.NoError(t, err)
			require.NotEmpty(t, got)

			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestWebsocketSubscriptionClientDeDuplication(t *testing.T) {
	serverDone := &sync.WaitGroup{}
	connectedClients := atomic.NewInt64(0)

	assertSubscription := func(ctx context.Context, conn *nhooyrwebsocket.Conn, subscriptionID int) {
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, nhooyrwebsocket.MessageText, msgType)
		assert.Equal(t, fmt.Sprintf(`{"type":"start","id":"%d","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, subscriptionID), string(data))
	}

	assertSendMessages := func(ctx context.Context, conn *nhooyrwebsocket.Conn, subscriptionID int) {
		err := conn.Write(ctx, nhooyrwebsocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"first"}}}}`, subscriptionID)))
		assert.NoError(t, err)
		err = conn.Write(ctx, nhooyrwebsocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"second"}}}}`, subscriptionID)))
		assert.NoError(t, err)
		err = conn.Write(ctx, nhooyrwebsocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"third"}}}}`, subscriptionID)))
		assert.NoError(t, err)
	}

	assertInitAck := func(ctx context.Context, conn *nhooyrwebsocket.Conn) {
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, nhooyrwebsocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(ctx, nhooyrwebsocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
	}

	assertReceiveMessages := func(next chan []byte) {
		first := <-next
		second := <-next
		third := <-next
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, string(third))
	}

	assertStop := func(ctx context.Context, conn *nhooyrwebsocket.Conn, subscriptionID ...int) {
		var receivedIDs []int
		expectedSum := 0
		actualSum := 0
		for _, expected := range subscriptionID {
			expectedSum += expected
			msgType, data, err := conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, nhooyrwebsocket.MessageText, msgType)
			messageType, err := jsonparser.GetString(data, "type")
			assert.NoError(t, err)
			assert.Equal(t, "stop", messageType)
			idStr, err := jsonparser.GetString(data, "id")
			assert.NoError(t, err)
			id, err := strconv.Atoi(idStr)
			assert.NoError(t, err)
			receivedIDs = append(receivedIDs, id)
			actualSum += id
		}
		assert.Len(t, receivedIDs, 4)
		assert.Equal(t, expectedSum, actualSum)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverDone.Add(1)
		defer serverDone.Done()
		conn, err := nhooyrwebsocket.Accept(w, r, nil)
		assert.NoError(t, err)
		connectedClients.Inc()
		defer connectedClients.Dec()

		assertInitAck(r.Context(), conn)

		assertSubscription(r.Context(), conn, 1)
		assertSendMessages(r.Context(), conn, 1)

		assertSubscription(r.Context(), conn, 2)
		assertSubscription(r.Context(), conn, 3)
		assertSubscription(r.Context(), conn, 4)

		assertSendMessages(r.Context(), conn, 2)
		assertSendMessages(r.Context(), conn, 3)
		assertSendMessages(r.Context(), conn, 4)

		assertStop(r.Context(), conn, 1, 2, 3, 4)
	}))
	defer server.Close()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	clientsDone := &sync.WaitGroup{}

	next := make(chan []byte)
	ctx, clientCancel := context.WithCancel(context.Background())
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	assertReceiveMessages(next)

	for i := 0; i < 3; i++ {
		clientsDone.Add(1)
		next := make(chan []byte)

		ctx, cancel := context.WithCancel(context.Background())

		err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
		}, next)
		assert.NoError(t, err)
		go func(next chan []byte, cancel func()) {
			assertReceiveMessages(next)
			cancel()
			clientsDone.Done()
		}(next, cancel)
	}

	clientCancel()

	serverDone.Wait()
	clientsDone.Wait()
	assert.Eventuallyf(t, func() bool {
		return connectedClients.Load() == 0
	}, time.Second, time.Millisecond, "clients not 0")
}

func TestWebsocketSubscriptionClientImmediateClientCancel(t *testing.T) {
	serverInvocations := atomic.NewInt64(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverInvocations.Inc()
	}))
	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())
	clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.Error(t, err)
	assert.Eventuallyf(t, func() bool {
		return serverInvocations.Load() == 0
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
	assert.Eventuallyf(t, func() bool {
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}

func TestWebsocketSubscriptionClientWithServerDisconnect(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := nhooyrwebsocket.Accept(w, r, nil)
		assert.NoError(t, err)
		ctx := context.Background()
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, nhooyrwebsocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), nhooyrwebsocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, nhooyrwebsocket.MessageText, msgType)
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), nhooyrwebsocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), nhooyrwebsocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), nhooyrwebsocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
		assert.NoError(t, err)

		_, _, err = conn.Read(ctx)
		assert.Error(t, err)
		close(serverDone)
	}))
	defer server.Close()
	ctx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
		WithWSSubProtocol(ProtocolGraphQLWS),
	)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	first := <-next
	second := <-next
	third := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, string(third))
	serverCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	assert.Eventuallyf(t, func() bool {
		client.handlersMu.Lock()
		defer client.handlersMu.Unlock()
		return len(client.handlers) == 0
	}, time.Second, time.Millisecond, "client handlers not 0")
}

func TestSubscriptionClientDynamicHeadersIsolation(t *testing.T) {
	var (
		mu              sync.Mutex
		receivedHeaders []string
		connectionCount int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders = append(receivedHeaders, r.Header.Get("Authorization"))
		connectionCount++
		mu.Unlock()

		c, err := nhooyrwebsocket.Accept(w, r, &nhooyrwebsocket.AcceptOptions{
			Subprotocols: []string{"graphql-ws"},
		})
		if err != nil {
			return
		}
		defer c.Close(nhooyrwebsocket.StatusInternalError, "closing")

		ackMsg := `{"type":"connection_ack"}`
		_ = c.Write(context.Background(), nhooyrwebsocket.MessageText, []byte(ackMsg))

		// Keep connection alive to simulate an active subscription
		ctx := context.Background()
		for {
			_, _, err := c.Read(ctx)
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, context.Background())

	userTokens := []string{"user-1-token", "user-2-token"}

	options := GraphQLSubscriptionOptions{
		URL:  wsURL,
		Body: GraphQLBody{Query: `{"query":"subscription { messageAdded { id text } }"}`},
	}

	for _, token := range userTokens {
		modifier := func(header http.Header) {
			header.Set("Authorization", token)
		}

		ctx := postprocess.SetHeaderModifier(context.Background(), modifier)
		next := make(chan []byte)
		err := client.Subscribe(ctx, options, next)
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond) // Wait for connection to establish
	}

	mu.Lock()
	defer mu.Unlock()

	// Without the fix, connectionCount would be 1 because the second request
	// would reuse the first connection due to identical static hashes.
	// With the fix, connectionCount should be 2.
	tokenLen := len(userTokens)
	assert.Equal(t, tokenLen, connectionCount, "Expected two separate WebSocket connections to be established")
	assert.Len(t, receivedHeaders, tokenLen)

	for _, token := range userTokens {
		assert.Contains(t, receivedHeaders, token)
	}
}

type connectionInitTokenKey struct{}

// acceptGraphQLWSServer starts a websocket server that negotiates the graphql-ws protocol, records
// the connection_init message of every connection it accepts, acknowledges it and then keeps the
// connection open so a second subscription can be multiplexed onto it.
func acceptGraphQLWSServer(t *testing.T, record func(initMessage string)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := nhooyrwebsocket.Accept(w, r, &nhooyrwebsocket.AcceptOptions{
			Subprotocols: []string{ProtocolGraphQLWS},
		})
		if err != nil {
			return
		}
		defer conn.Close(nhooyrwebsocket.StatusInternalError, "closing")

		_, initMessage, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		record(string(initMessage))

		if err := conn.Write(r.Context(), nhooyrwebsocket.MessageText, []byte(`{"type":"connection_ack"}`)); err != nil {
			return
		}

		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				return
			}
		}
	}))
}

// TestSubscriptionClientConnectionInitIsolation guards against multiplexing two requests onto the
// same upstream websocket when only their connection_init payloads differ. That payload is produced
// by OnWsConnectionInitCallback, which is handed the request context precisely so it can forward the
// calling user's credentials - reusing a connection that was authenticated with somebody else's
// payload puts one user's subscription on another user's upstream session.
func TestSubscriptionClientConnectionInitIsolation(t *testing.T) {
	var (
		mu            sync.Mutex
		receivedInits []string
	)

	server := acceptGraphQLWSServer(t, func(initMessage string) {
		mu.Lock()
		receivedInits = append(receivedInits, initMessage)
		mu.Unlock()
	})
	defer server.Close()

	var callback OnWsConnectionInitCallback = func(ctx context.Context, url string, header http.Header) (json.RawMessage, error) {
		token, _ := ctx.Value(connectionInitTokenKey{}).(string)
		return json.RawMessage(fmt.Sprintf(`{"token":%q}`, token)), nil
	}

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, context.Background(),
		WithOnWsConnectionInitCallback(&callback),
	)

	options := GraphQLSubscriptionOptions{
		URL:  "ws" + server.URL[4:],
		Body: GraphQLBody{Query: `subscription {messageAdded(roomName: "room"){text}}`},
	}

	userTokens := []string{"user-1-token", "user-2-token"}
	for _, token := range userTokens {
		ctx := context.WithValue(context.Background(), connectionInitTokenKey{}, token)
		next := make(chan []byte)
		require.NoError(t, client.Subscribe(ctx, options, next))
	}

	assert.Eventuallyf(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedInits) == len(userTokens)
	}, time.Second, time.Millisecond*10, "expected one upstream connection per connection_init payload")

	mu.Lock()
	defer mu.Unlock()

	for _, token := range userTokens {
		assert.Contains(t, receivedInits, fmt.Sprintf(`{"type":"connection_init","payload":{"token":"%s"}}`, token))
	}
}

// TestSubscriptionClientDoesNotPinNegotiatedSubProtocol guards against one connection's negotiated
// sub-protocol being written back onto the shared client - it would become the client wide default
// and narrow the protocols offered to every upstream dialled afterwards.
func TestSubscriptionClientDoesNotPinNegotiatedSubProtocol(t *testing.T) {
	server := acceptGraphQLWSServer(t, func(string) {})
	defer server.Close()

	// No WithWSSubProtocol: the client offers both protocols and lets each connection negotiate.
	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, context.Background())

	next := make(chan []byte)
	require.NoError(t, client.Subscribe(context.Background(), GraphQLSubscriptionOptions{
		URL:  "ws" + server.URL[4:],
		Body: GraphQLBody{Query: `subscription {messageAdded(roomName: "room"){text}}`},
	}, next))

	assert.Empty(t, client.wsSubProtocol,
		"the protocol negotiated for a single connection must not be pinned onto the shared client")
}

// TestConnectionKey covers the key that decides whether an upstream websocket connection is reused.
// It has to tell apart every part of the effective connection descriptor - anything it misses is a
// connection shared between two requests that should not share one - while staying stable for
// descriptors that are equal but were built in a different order.
func TestConnectionKey(t *testing.T) {
	const connectionInit = `{"type":"connection_init","payload":{"token":"A"}}`

	baseOptions := func() GraphQLSubscriptionOptions {
		return GraphQLSubscriptionOptions{
			URL:    "ws://example.com/graphql",
			Header: http.Header{"Authorization": {"Bearer A"}},
		}
	}

	key := func(t *testing.T, options GraphQLSubscriptionOptions, initMessage string) string {
		t.Helper()

		result, err := connectionKey(options, []byte(initMessage))
		require.NoError(t, err)
		return result
	}

	base := key(t, baseOptions(), connectionInit)

	t.Run("is stable for the same descriptor", func(t *testing.T) {
		assert.Equal(t, base, key(t, baseOptions(), connectionInit))
	})

	t.Run("does not depend on the order the headers were set in", func(t *testing.T) {
		first := http.Header{}
		first.Set("Authorization", "Bearer A")
		first.Set("X-Tenant", "tenant-1")

		second := http.Header{}
		second.Set("X-Tenant", "tenant-1")
		second.Set("Authorization", "Bearer A")

		firstOptions, secondOptions := baseOptions(), baseOptions()
		firstOptions.Header, secondOptions.Header = first, second

		assert.Equal(t, key(t, firstOptions, connectionInit), key(t, secondOptions, connectionInit))
	})

	t.Run("differs by url", func(t *testing.T) {
		options := baseOptions()
		options.URL = "ws://example.com/other-graphql"

		assert.NotEqual(t, base, key(t, options, connectionInit))
	})

	t.Run("differs by header value", func(t *testing.T) {
		options := baseOptions()
		options.Header = http.Header{"Authorization": {"Bearer B"}}

		assert.NotEqual(t, base, key(t, options, connectionInit))
	})

	t.Run("differs by an additional header", func(t *testing.T) {
		options := baseOptions()
		options.Header = http.Header{"Authorization": {"Bearer A"}, "X-Tenant": {"tenant-1"}}

		assert.NotEqual(t, base, key(t, options, connectionInit))
	})

	t.Run("differs by connection init payload", func(t *testing.T) {
		assert.NotEqual(t, base, key(t, baseOptions(), `{"type":"connection_init","payload":{"token":"B"}}`))
	})

	t.Run("handles options without headers", func(t *testing.T) {
		options := baseOptions()
		options.Header = nil

		withoutHeaders := key(t, options, connectionInit)
		assert.NotEqual(t, base, withoutHeaders)
		assert.Equal(t, withoutHeaders, key(t, options, connectionInit))
	})
}
