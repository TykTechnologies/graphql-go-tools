package graphql_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	"github.com/coder/websocket"
	ll "github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
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

	assertSubscription := func(ctx context.Context, conn *websocket.Conn, subscriptionID int) {
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, fmt.Sprintf(`{"type":"start","id":"%d","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, subscriptionID), string(data))
	}

	assertSendMessages := func(ctx context.Context, conn *websocket.Conn, subscriptionID int) {
		err := conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"first"}}}}`, subscriptionID)))
		assert.NoError(t, err)
		err = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"second"}}}}`, subscriptionID)))
		assert.NoError(t, err)
		err = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"data","id":"%d","payload":{"data":{"messageAdded":{"text":"third"}}}}`, subscriptionID)))
		assert.NoError(t, err)
	}

	assertInitAck := func(ctx context.Context, conn *websocket.Conn) {
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
	}

	assertReceiveMessages := func(t *testing.T, updater *testSubscriptionUpdater) {
		t.Helper()
		updater.AwaitUpdates(t, time.Second, 3)
		assert.Equal(t, 3, len(updater.updates))
		assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
		assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
		assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
	}

	assertStop := func(ctx context.Context, conn *websocket.Conn, subscriptionID ...int) {
		var receivedIDs []int
		expectedSum := 0
		actualSum := 0
		for _, expected := range subscriptionID {
			expectedSum += expected
			msgType, data, err := conn.Read(ctx)
			assert.NoError(t, err)
			assert.Equal(t, websocket.MessageText, msgType)
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
		conn, err := websocket.Accept(w, r, nil)
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

	updater := &testSubscriptionUpdater{}

	ctx, clientCancel := context.WithCancel(context.Background())
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, updater)
	assert.NoError(t, err)
	assertReceiveMessages(t, updater)

	for i := 0; i < 3; i++ {
		clientsDone.Add(1)

		updater := &testSubscriptionUpdater{}

		ctx, cancel := context.WithCancel(context.Background())

		err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
			URL: server.URL,
			Body: GraphQLBody{
				Query: `subscription {messageAdded(roomName: "room"){text}}`,
			},
		}, updater)
		assert.NoError(t, err)
		go func(updater *testSubscriptionUpdater, cancel func()) {
			assertReceiveMessages(t, updater)
			cancel()
			clientsDone.Done()
		}(updater, cancel)
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
	updater := &testSubscriptionUpdater{}
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, updater)
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
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		ctx := context.Background()
		msgType, data, err := conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
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
	updater := &testSubscriptionUpdater{}
	err := client.Subscribe(resolve.NewContext(ctx), GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, updater)
	assert.NoError(t, err)
	updater.AwaitUpdates(t, time.Second, 3)
	assert.Equal(t, 3, len(updater.updates))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, updater.updates[0])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, updater.updates[1])
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, updater.updates[2])
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

type connectionInitTokenKey struct{}

// acceptGraphQLWSServer starts a websocket server that negotiates the graphql-ws protocol, records
// the connection_init message of every connection it accepts, acknowledges it and then keeps the
// connection open so a second subscription can be multiplexed onto it.
func acceptGraphQLWSServer(t *testing.T, record func(initMessage string)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{ProtocolGraphQLWS},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusInternalError, "closing")

		_, initMessage, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		record(string(initMessage))

		if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`)); err != nil {
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
		require.NoError(t, client.Subscribe(resolve.NewContext(ctx), options, &testSubscriptionUpdater{}))
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

	require.NoError(t, client.Subscribe(resolve.NewContext(context.Background()), GraphQLSubscriptionOptions{
		URL:  "ws" + server.URL[4:],
		Body: GraphQLBody{Query: `subscription {messageAdded(roomName: "room"){text}}`},
	}, &testSubscriptionUpdater{}))

	assert.Empty(t, client.wsSubProtocol,
		"the protocol negotiated for a single connection must not be pinned onto the shared client")
}

// TestConnectionKey covers the key that decides whether an upstream websocket connection is reused.
// It has to tell apart every part of the effective connection descriptor - including the client
// headers that will be forwarded to the subgraph - while staying stable for descriptors that are
// equal but were built in a different order.
func TestConnectionKey(t *testing.T) {
	const connectionInit = `{"type":"connection_init","payload":{"token":"A"}}`

	newCtx := func(clientHeaders http.Header) *resolve.Context {
		ctx := resolve.NewContext(context.Background())
		ctx.Request.Header = clientHeaders
		return ctx
	}

	baseOptions := func() GraphQLSubscriptionOptions {
		return GraphQLSubscriptionOptions{
			URL:    "ws://example.com/graphql",
			Header: http.Header{"Authorization": {"Bearer A"}},
		}
	}

	key := func(t *testing.T, ctx *resolve.Context, options GraphQLSubscriptionOptions, initMessage string) string {
		t.Helper()

		result, err := connectionKey(ctx, options, []byte(initMessage))
		require.NoError(t, err)
		return result
	}

	base := key(t, newCtx(nil), baseOptions(), connectionInit)

	t.Run("is stable for the same descriptor", func(t *testing.T) {
		assert.Equal(t, base, key(t, newCtx(nil), baseOptions(), connectionInit))
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

		assert.Equal(t,
			key(t, newCtx(nil), firstOptions, connectionInit),
			key(t, newCtx(nil), secondOptions, connectionInit),
		)
	})

	t.Run("differs by url", func(t *testing.T) {
		options := baseOptions()
		options.URL = "ws://example.com/other-graphql"

		assert.NotEqual(t, base, key(t, newCtx(nil), options, connectionInit))
	})

	t.Run("differs by header value", func(t *testing.T) {
		options := baseOptions()
		options.Header = http.Header{"Authorization": {"Bearer B"}}

		assert.NotEqual(t, base, key(t, newCtx(nil), options, connectionInit))
	})

	t.Run("differs by an additional header", func(t *testing.T) {
		options := baseOptions()
		options.Header = http.Header{"Authorization": {"Bearer A"}, "X-Tenant": {"tenant-1"}}

		assert.NotEqual(t, base, key(t, newCtx(nil), options, connectionInit))
	})

	t.Run("differs by connection init payload", func(t *testing.T) {
		assert.NotEqual(t, base, key(t, newCtx(nil), baseOptions(), `{"type":"connection_init","payload":{"token":"B"}}`))
	})

	t.Run("handles options without headers", func(t *testing.T) {
		options := baseOptions()
		options.Header = nil

		withoutHeaders := key(t, newCtx(nil), options, connectionInit)
		assert.NotEqual(t, base, withoutHeaders)
		assert.Equal(t, withoutHeaders, key(t, newCtx(nil), options, connectionInit))
	})

	t.Run("forwarded client headers", func(t *testing.T) {
		forwardedByName := baseOptions()
		forwardedByName.ForwardedClientHeaderNames = []string{"X-Tenant"}

		forwardedByRegexp := baseOptions()
		forwardedByRegexp.ForwardedClientHeaderRegularExpressions = []*regexp.Regexp{regexp.MustCompile("^X-.*")}

		clientHeader := func(value string) http.Header {
			header := http.Header{}
			header.Set("X-Tenant", value)
			return header
		}

		t.Run("match when the forwarded value is the same", func(t *testing.T) {
			assert.Equal(t,
				key(t, newCtx(clientHeader("tenant-1")), forwardedByName, connectionInit),
				key(t, newCtx(clientHeader("tenant-1")), forwardedByName, connectionInit),
			)
		})

		t.Run("differ when the forwarded value differs", func(t *testing.T) {
			assert.NotEqual(t,
				key(t, newCtx(clientHeader("tenant-1")), forwardedByName, connectionInit),
				key(t, newCtx(clientHeader("tenant-2")), forwardedByName, connectionInit),
			)
		})

		t.Run("differ when a header matched by a regular expression differs", func(t *testing.T) {
			assert.NotEqual(t,
				key(t, newCtx(clientHeader("tenant-1")), forwardedByRegexp, connectionInit),
				key(t, newCtx(clientHeader("tenant-2")), forwardedByRegexp, connectionInit),
			)
		})

		t.Run("ignore client headers that are not forwarded", func(t *testing.T) {
			assert.Equal(t,
				key(t, newCtx(clientHeader("tenant-1")), baseOptions(), connectionInit),
				key(t, newCtx(clientHeader("tenant-2")), baseOptions(), connectionInit),
			)
		})
	})
}
