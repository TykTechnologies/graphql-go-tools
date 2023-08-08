package subscription

//go:generate mockgen -destination=handler_mock_test.go -package=subscription . Protocol,EventHandler

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
)

var ErrCouldNotReadMessageFromClient = errors.New("could not read message from client")

// EventType can be used to define subscription events decoupled from any protocols.
type EventType int

const (
	EventTypeError EventType = iota
	EventTypeData
	EventTypeCompleted
	EventTypeConnectionTerminatedByClient
	EventTypeConnectionTerminatedByServer
	EventTypeConnectionError
)

// Protocol defines an interface for a subscription protocol decoupled from the underlying transport.
type Protocol interface {
	Handle(ctx context.Context, engine Engine, message []byte) error
	EventHandler() EventHandler
}

// EventHandler is an interface that handles subscription events.
type EventHandler interface {
	Emit(eventType EventType, id string, data []byte, err error)
}

// UniversalProtocolHandlerOptions is struct that defines options for the UniversalProtocolHandler.
type UniversalProtocolHandlerOptions struct {
	Logger                           abstractlogger.Logger
	CustomSubscriptionUpdateInterval time.Duration
	CustomEngine                     Engine
}

// UniversalProtocolHandler can handle any protocol by using the Protocol interface.
type UniversalProtocolHandler struct {
	logger   abstractlogger.Logger
	client   TransportClient
	protocol Protocol
	engine   Engine
}

// NewUniversalProtocolHandler creates a new UniversalProtocolHandler.
func NewUniversalProtocolHandler(client TransportClient, protocol Protocol, executorPool ExecutorPool) (*UniversalProtocolHandler, error) {
	options := UniversalProtocolHandlerOptions{
		Logger: abstractlogger.Noop{},
	}

	return NewUniversalProtocolHandlerWithOptions(client, protocol, executorPool, options)
}

// NewUniversalProtocolHandlerWithOptions creates a new UniversalProtocolHandler. It requires an option struct.
func NewUniversalProtocolHandlerWithOptions(client TransportClient, protocol Protocol, executorPool ExecutorPool, options UniversalProtocolHandlerOptions) (*UniversalProtocolHandler, error) {
	handler := UniversalProtocolHandler{
		logger:   abstractlogger.Noop{},
		client:   client,
		protocol: protocol,
	}

	if options.Logger != nil {
		handler.logger = options.Logger
	}

	if options.CustomEngine != nil {
		handler.engine = options.CustomEngine
	} else {
		engine := ExecutorEngine{
			logger:           handler.logger,
			subCancellations: subscriptionCancellations{},
			executorPool:     executorPool,
			bufferPool: &sync.Pool{
				New: func() interface{} {
					writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
					return &writer
				},
			},
		}

		if options.CustomSubscriptionUpdateInterval != 0 {
			engine.subscriptionUpdateInterval = options.CustomSubscriptionUpdateInterval
		} else {
			subscriptionUpdateInterval, err := time.ParseDuration(DefaultSubscriptionUpdateInterval)
			if err != nil {
				return nil, err
			}
			engine.subscriptionUpdateInterval = subscriptionUpdateInterval
		}
		handler.engine = &engine
	}

	return &handler, nil
}

// Handle will handle the subscription logic and forward messages to the actual protocol handler.
func (u *UniversalProtocolHandler) Handle(ctx context.Context) {
	ctxWithCancel, cancel := context.WithCancel(ctx)
	defer func() {
		err := u.engine.TerminateAllSubscriptions(u.protocol.EventHandler())
		if err != nil {
			u.logger.Error("subscription.UniversalProtocolHandler.Handle: on terminate connections",
				abstractlogger.Error(err),
			)
		}
		cancel()
	}()

	for {
		if !u.client.IsConnected() {
			u.logger.Debug("subscription.UniversalProtocolHandler.Handle: on client is connected check",
				abstractlogger.String("message", "client has disconnected"),
			)

			return
		}

		message, err := u.client.ReadBytesFromClient()
		if errors.Is(err, ErrTransportClientClosedConnection) {
			u.logger.Debug("subscription.UniversalProtocolHandler.Handle: reading from a closed connection")
			return
		} else if err != nil {
			u.logger.Error("subscription.UniversalProtocolHandler.Handle: on reading bytes from client",
				abstractlogger.Error(err),
				abstractlogger.ByteString("message", message),
			)

			u.protocol.EventHandler().Emit(EventTypeConnectionError, "", nil, ErrCouldNotReadMessageFromClient)
		} else if len(message) > 0 {
			err := u.protocol.Handle(ctxWithCancel, u.engine, message)
			if err != nil {
				u.logger.Error("subscription.UniversalProtocolHandler.Handle: on protocol handling message",
					abstractlogger.Error(err),
				)
			}
		}

		select {
		case <-ctxWithCancel.Done():
			return
		default:
			continue
		}
	}
}
