package graphql_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/textproto"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/coder/websocket"
	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

const ackWaitTimeout = 30 * time.Second

// SubscriptionClient allows running multiple subscriptions via the same WebSocket either SSE connection
// It takes care of de-duplicating connections to the same origin under certain circumstances
// If the full effective connection descriptor is identical, an existing connection is re-used.
type SubscriptionClient struct {
	streamingClient            *http.Client
	httpClient                 *http.Client
	engineCtx                  context.Context
	log                        abstractlogger.Logger
	handlers                   map[string]ConnectionHandler
	handlersMu                 sync.Mutex
	wsSubProtocol              string
	onWsConnectionInitCallback *OnWsConnectionInitCallback

	readTimeout time.Duration
}

type Options func(options *opts)

func WithLogger(log abstractlogger.Logger) Options {
	return func(options *opts) {
		options.log = log
	}
}

func WithReadTimeout(timeout time.Duration) Options {
	return func(options *opts) {
		options.readTimeout = timeout
	}
}

func WithWSSubProtocol(protocol string) Options {
	return func(options *opts) {
		options.wsSubProtocol = protocol
	}
}

func WithOnWsConnectionInitCallback(callback *OnWsConnectionInitCallback) Options {
	return func(options *opts) {
		options.onWsConnectionInitCallback = callback
	}
}

type opts struct {
	readTimeout                time.Duration
	log                        abstractlogger.Logger
	wsSubProtocol              string
	onWsConnectionInitCallback *OnWsConnectionInitCallback
}

// GraphQLSubscriptionClientFactory abstracts the way of creating a new GraphQLSubscriptionClient.
// This can be very handy for testing purposes.
type GraphQLSubscriptionClientFactory interface {
	NewSubscriptionClient(httpClient, streamingClient *http.Client, engineCtx context.Context, options ...Options) GraphQLSubscriptionClient
}

type DefaultSubscriptionClientFactory struct{}

func (d *DefaultSubscriptionClientFactory) NewSubscriptionClient(httpClient, streamingClient *http.Client, engineCtx context.Context, options ...Options) GraphQLSubscriptionClient {
	return NewGraphQLSubscriptionClient(httpClient, streamingClient, engineCtx, options...)
}

func NewGraphQLSubscriptionClient(httpClient, streamingClient *http.Client, engineCtx context.Context, options ...Options) *SubscriptionClient {
	op := &opts{
		readTimeout: time.Second,
		log:         abstractlogger.NoopLogger,
	}
	for _, option := range options {
		option(op)
	}
	return &SubscriptionClient{
		httpClient:                 httpClient,
		streamingClient:            streamingClient,
		engineCtx:                  engineCtx,
		handlers:                   make(map[string]ConnectionHandler),
		log:                        op.log,
		readTimeout:                op.readTimeout,
		wsSubProtocol:              op.wsSubProtocol,
		onWsConnectionInitCallback: op.onWsConnectionInitCallback,
	}
}

// Subscribe initiates a new GraphQL Subscription with the origin
// If an existing WS connection with the same ID (Hash) exists, it is being re-used
// If connection protocol is SSE, a new connection is always created
// If no connection exists, the client initiates a new one
func (c *SubscriptionClient) Subscribe(reqCtx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	// Dynamically apply the header modifier to the options BEFORE any connection or hashing logic.
	// This ensures that dynamic headers are included in options.Header,
	// resulting in a unique hash for different users, while allowing the same user to multiplex.
	if modifier := postprocess.GetHeaderModifier(reqCtx.Context()); modifier != nil {
		if options.Header == nil {
			options.Header = make(http.Header)
		}
		modifier(options.Header)
	}

	if options.UseSSE {
		return c.subscribeSSE(reqCtx, options, updater)
	}

	return c.subscribeWS(reqCtx, options, updater)
}

var (
	withSSE           = []byte(`sse:true`)
	withSSEMethodPost = []byte(`sse_method_post:true`)
)

func (c *SubscriptionClient) UniqueRequestID(ctx *resolve.Context, options GraphQLSubscriptionOptions, hash *xxhash.Digest) (err error) {
	if options.UseSSE {
		_, err = hash.Write(withSSE)
		if err != nil {
			return err
		}
	}
	if options.SSEMethodPost {
		_, err = hash.Write(withSSEMethodPost)
		if err != nil {
			return err
		}
	}
	return c.requestHash(ctx, options, hash)
}

func (c *SubscriptionClient) subscribeSSE(reqCtx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if c.streamingClient == nil {
		return fmt.Errorf("streaming http client is nil")
	}

	sub := Subscription{
		ctx:     reqCtx.Context(),
		options: options,
		updater: updater,
	}

	handler := newSSEConnectionHandler(reqCtx, c.streamingClient, options, c.log)

	go func() {
		handler.StartBlocking(sub)
	}()

	return nil
}

func (c *SubscriptionClient) subscribeWS(reqCtx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client is nil")
	}

	sub := Subscription{
		ctx:     reqCtx.Context(),
		options: options,
		updater: updater,
	}

	connectionInitMessage, err := c.getConnectionInitMessage(reqCtx.Context(), options.URL, options.Header)
	if err != nil {
		return err
	}
	if len(options.InitialPayload) > 0 {
		connectionInitMessage, err = jsonparser.Set(connectionInitMessage, options.InitialPayload, "payload")
		if err != nil {
			return err
		}
	}
	if options.Body.Extensions != nil {
		connectionInitMessage, err = jsonparser.Set(connectionInitMessage, options.Body.Extensions, "payload", "extensions")
		if err != nil {
			return err
		}
	}
	handlerID, err := connectionKey(reqCtx, options, connectionInitMessage)
	if err != nil {
		return err
	}

	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	handler, exists := c.handlers[handlerID]
	if exists {
		select {
		case handler.SubscribeCH() <- sub:
		case <-reqCtx.Context().Done():
		}
		return nil
	}

	handler, err = c.newWSConnectionHandler(reqCtx.Context(), options, connectionInitMessage)
	if err != nil {
		return err
	}

	c.handlers[handlerID] = handler

	go func(handlerID string) {
		handler.StartBlocking(sub)
		c.handlersMu.Lock()
		delete(c.handlers, handlerID)
		c.handlersMu.Unlock()
	}(handlerID)

	return nil
}

func connectionKey(ctx *resolve.Context, options GraphQLSubscriptionOptions, connectionInitMessage []byte) (string, error) {
	forwardedHeaders := make(http.Header)
	for _, headerName := range options.ForwardedClientHeaderNames {
		canonicalName := textproto.CanonicalMIMEHeaderKey(headerName)
		forwardedHeaders[canonicalName] = append([]string(nil), ctx.Request.Header[canonicalName]...)
	}
	for _, headerRegexp := range options.ForwardedClientHeaderRegularExpressions {
		for headerName, values := range ctx.Request.Header {
			if headerRegexp.MatchString(headerName) {
				canonicalName := textproto.CanonicalMIMEHeaderKey(headerName)
				forwardedHeaders[canonicalName] = append([]string(nil), values...)
			}
		}
	}
	descriptor := struct {
		URL            string
		Header         http.Header
		Forwarded      http.Header
		ConnectionInit string
	}{
		URL:            options.URL,
		Header:         options.Header,
		Forwarded:      forwardedHeaders,
		ConnectionInit: string(connectionInitMessage),
	}
	key, err := json.Marshal(descriptor)
	if err != nil {
		return "", err
	}
	return string(key), nil
}

// requestHash contributes the subscription descriptor to the resolver's candidate trigger ID.
func (c *SubscriptionClient) requestHash(ctx *resolve.Context, options GraphQLSubscriptionOptions, xxh *xxhash.Digest) (err error) {
	if _, err = xxh.WriteString(options.URL); err != nil {
		return err
	}
	if err := options.Header.Write(xxh); err != nil {
		return err
	}
	// Make sure any header that will be forwarded to the subgraph
	// is hashed to create the handlerID, this way requests with
	// different headers will use separate connections.
	for _, headerName := range options.ForwardedClientHeaderNames {
		if _, err = xxh.WriteString(headerName); err != nil {
			return err
		}
		for _, val := range ctx.Request.Header[textproto.CanonicalMIMEHeaderKey(headerName)] {
			if _, err = xxh.WriteString(val); err != nil {
				return err
			}
		}
	}
	for _, headerRegexp := range options.ForwardedClientHeaderRegularExpressions {
		if _, err = xxh.WriteString(headerRegexp.String()); err != nil {
			return err
		}
		for headerName, values := range ctx.Request.Header {
			if headerRegexp.MatchString(headerName) {
				for _, val := range values {
					if _, err = xxh.WriteString(val); err != nil {
						return err
					}
				}
			}
		}
	}
	if len(ctx.InitialPayload) > 0 {
		if _, err = xxh.Write(ctx.InitialPayload); err != nil {
			return err
		}
	}
	if options.Body.Extensions != nil {
		if _, err = xxh.Write(options.Body.Extensions); err != nil {
			return err
		}
	}
	return nil
}

func (c *SubscriptionClient) newWSConnectionHandler(reqCtx context.Context, options GraphQLSubscriptionOptions, connectionInitMessage []byte) (ConnectionHandler, error) {
	subProtocols := []string{ProtocolGraphQLWS, ProtocolGraphQLTWS}
	if c.wsSubProtocol != "" {
		subProtocols = []string{c.wsSubProtocol}
	}

	conn, upgradeResponse, err := websocket.Dial(reqCtx, options.URL, &websocket.DialOptions{
		HTTPClient:      c.httpClient,
		HTTPHeader:      options.Header,
		CompressionMode: websocket.CompressionDisabled,
		Subprotocols:    subProtocols,
	})
	if err != nil {
		return nil, err
	}
	// Disable the maximum message size limit. Don't use MaxInt64 since
	// the github.com/coder/websocket doesn't handle it correctly on 32 bit systems.
	conn.SetReadLimit(math.MaxInt32)
	if upgradeResponse.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("upgrade unsuccessful")
	}

	// init + ack
	err = conn.Write(reqCtx, websocket.MessageText, connectionInitMessage)
	if err != nil {
		return nil, err
	}

	if err := waitForAck(reqCtx, conn); err != nil {
		return nil, err
	}

	protocol := c.wsSubProtocol
	if protocol == "" {
		protocol = conn.Subprotocol()
	}
	switch protocol {
	case ProtocolGraphQLWS:
		return newGQLWSConnectionHandler(c.engineCtx, conn, c.readTimeout, c.log), nil
	case ProtocolGraphQLTWS:
		return newGQLTWSConnectionHandler(c.engineCtx, conn, c.readTimeout, c.log), nil
	default:
		return nil, fmt.Errorf("unknown protocol %s", protocol)
	}
}

func (c *SubscriptionClient) getConnectionInitMessage(ctx context.Context, url string, header http.Header) ([]byte, error) {
	if c.onWsConnectionInitCallback == nil {
		return connectionInitMessage, nil
	}

	callback := *c.onWsConnectionInitCallback

	payload, err := callback(ctx, url, header)
	if err != nil {
		return nil, err
	}

	if len(payload) == 0 {
		return connectionInitMessage, nil
	}

	msg, err := jsonparser.Set(connectionInitMessage, payload, "payload")
	if err != nil {
		return nil, err
	}

	return msg, nil
}

type ConnectionHandler interface {
	StartBlocking(sub Subscription)
	SubscribeCH() chan<- Subscription
}

type Subscription struct {
	ctx     context.Context
	options GraphQLSubscriptionOptions
	updater resolve.SubscriptionUpdater
}

func waitForAck(ctx context.Context, conn *websocket.Conn) error {
	timer := time.NewTimer(ackWaitTimeout)
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout while waiting for connection_ack")
		default:
		}

		msgType, msg, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if msgType != websocket.MessageText {
			return fmt.Errorf("unexpected message type")
		}

		respType, err := jsonparser.GetString(msg, "type")
		if err != nil {
			return err
		}

		switch respType {
		case messageTypeConnectionKeepAlive:
			continue
		case messageTypePing:
			err := conn.Write(ctx, websocket.MessageText, []byte(pongMessage))
			if err != nil {
				return fmt.Errorf("failed to send pong message: %w", err)
			}

			continue
		case messageTypeConnectionAck:
			return nil
		default:
			return fmt.Errorf("expected connection_ack or ka, got %s", respType)
		}
	}
}
