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

	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
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

		ctx := context.WithValue(context.Background(), resolve.HeaderModifierContextKey, postprocess.HeaderModifier(modifier))
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
