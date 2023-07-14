package websocket

import (
	"context"
	"net"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/subscription"
)

type Protocol string

const (
	ProtocolGraphQLWS Protocol = "graphql-ws"
)

var DefaultProtocol Protocol = ProtocolGraphQLWS

type HandleOptions struct {
	Logger                           abstractlogger.Logger
	WebSocketInitFunc                InitFunc
	CustomClient                     subscription.TransportClient
	CustomKeepAliveInterval          time.Duration
	CustomSubscriptionUpdateInterval time.Duration
	CustomSubscriptionEngine         subscription.Engine
}

type HandleOptionFunc func(opts *HandleOptions)

func WithLogger(logger abstractlogger.Logger) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.Logger = logger
	}
}

func WithInitFunc(initFunc InitFunc) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.WebSocketInitFunc = initFunc
	}
}

func WithCustomClient(client subscription.TransportClient) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomClient = client
	}
}

func WithCustomKeepAliveInterval(keepAliveInterval time.Duration) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomKeepAliveInterval = keepAliveInterval
	}
}

func WithCustomSubscriptionUpdateInterval(subscriptionUpdateInterval time.Duration) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomSubscriptionUpdateInterval = subscriptionUpdateInterval
	}
}

func WithCustomSubscriptionEngine(subscriptionEngine subscription.Engine) HandleOptionFunc {
	return func(opts *HandleOptions) {
		opts.CustomSubscriptionEngine = subscriptionEngine
	}
}

func Handle(done chan bool, errChan chan error, conn net.Conn, executorPool subscription.ExecutorPool, options ...HandleOptionFunc) {
	definedOptions := HandleOptions{
		Logger: abstractlogger.Noop{},
	}

	for _, optionFunc := range options {
		optionFunc(&definedOptions)
	}

	HandleWithOptions(done, errChan, conn, executorPool, definedOptions)
}

func HandleWithOptions(done chan bool, errChan chan error, conn net.Conn, executorPool subscription.ExecutorPool, options HandleOptions) {
	// Use noop logger to prevent nil pointers if none was provided
	if options.Logger == nil {
		options.Logger = abstractlogger.Noop{}
	}

	defer func() {
		if err := conn.Close(); err != nil {
			options.Logger.Error("websocket.HandleWithOptions: on deferred closing connection",
				abstractlogger.String("message", "could not close connection to client"),
				abstractlogger.Error(err),
			)
		}
	}()

	var client subscription.TransportClient
	if options.CustomClient != nil {
		client = options.CustomClient
	} else {
		client = NewClient(options.Logger, conn)
	}

	protocolHandler, err := NewProtocolGraphQLWSHandlerWithOptions(client, ProtocolGraphQLWSHandlerOptions{
		Logger:                  options.Logger,
		WebSocketInitFunc:       options.WebSocketInitFunc,
		CustomKeepAliveInterval: options.CustomKeepAliveInterval,
	})
	if err != nil {
		options.Logger.Error("websocket.HandleWithOptions: on protocol handler creation",
			abstractlogger.String("message", "could not create protocol handler"),
			abstractlogger.String("protocol", string(DefaultProtocol)),
			abstractlogger.Error(err),
		)

		errChan <- err
		return
	}

	subscriptionHandler, err := subscription.NewUniversalProtocolHandlerWithOptions(client, protocolHandler, executorPool, subscription.UniversalProtocolHandlerOptions{
		Logger:                           options.Logger,
		CustomSubscriptionUpdateInterval: options.CustomSubscriptionUpdateInterval,
		CustomEngine:                     options.CustomSubscriptionEngine,
	})
	if err != nil {
		options.Logger.Error("websocket.HandleWithOptions: on subscription handler creation",
			abstractlogger.String("message", "could not create subscription handler"),
			abstractlogger.String("protocol", string(DefaultProtocol)),
			abstractlogger.Error(err),
		)

		errChan <- err
		return
	}

	close(done)
	subscriptionHandler.Handle(context.Background()) // Blocking
}
