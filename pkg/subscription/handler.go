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

type EventType int

const (
	EventTypeError = iota
	EventTypeData
	EventTypeCompleted
	EventTypeConnectionTerminate
	EventTypeConnectionError
)

type Protocol interface {
	Handle(ctx context.Context, engine Engine, message []byte) error
	EventHandler() EventHandler
}

type EventHandler interface {
	Emit(eventType EventType, id string, data []byte, err error)
}

type UniversalProtocolHandlerOptions struct {
	Logger                           abstractlogger.Logger
	CustomSubscriptionUpdateInterval time.Duration
	CustomEngine                     Engine
}

type UniversalProtocolHandler struct {
	logger abstractlogger.Logger
	// client will hold the subscription client implementation.
	client   TransportClient
	protocol Protocol
	engine   Engine
}

func NewUniversalProtocolHandler(client TransportClient, protocol Protocol, executorPool ExecutorPool) (*UniversalProtocolHandler, error) {
	options := UniversalProtocolHandlerOptions{
		Logger: abstractlogger.Noop{},
	}

	return NewUniversalProtocolHandlerWithOptions(client, protocol, executorPool, options)
}

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
	}

	return &handler, nil
}

func (u *UniversalProtocolHandler) Handle(ctx context.Context) {
	defer func() {
		err := u.engine.TerminateAllConnections(u.protocol.EventHandler())
		if err != nil {
			u.logger.Error("subscription.UniversalProtocolHandler.Handle: on terminate connections",
				abstractlogger.Error(err),
			)
		}
	}()

	for {
		if !u.client.IsConnected() {
			u.logger.Debug("subscription.UniversalProtocolHandler.Handle: on client is connected check",
				abstractlogger.String("message", "client has disconnected"),
			)

			return
		}

		message, err := u.client.ReadBytesFromClient()
		if err != nil {
			u.logger.Error("subscription.UniversalProtocolHandler.Handle: on reading bytes from client",
				abstractlogger.Error(err),
				abstractlogger.ByteString("message", message),
			)

			u.protocol.EventHandler().Emit(EventTypeConnectionError, "", nil, ErrCouldNotReadMessageFromClient)
		} else if len(message) > 0 {
			err := u.protocol.Handle(ctx, u.engine, message)
			if err != nil {
				u.logger.Error("subscription.UniversalProtocolHandler.Handle: on protocol handling message",
					abstractlogger.Error(err),
				)
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
			continue
		}
	}
}
