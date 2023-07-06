package subscription

//go:generate mockgen -destination=handler_mock_test.go -package=subscription . Protocol,EventHandler

import (
	"context"
	"time"

	"github.com/jensneuse/abstractlogger"
)

type EventType int

const (
	EventTypeNone EventType = iota
	EventTypeError
	EventTypeData
	EventTypeCompleted
)

type Protocol interface {
	Handle(ctx context.Context, message []byte) error
}

type EventHandler interface {
	Emit(eventType EventType, id string, data []byte, err error)
}

type UniversalProtocolHandler struct {
	logger abstractlogger.Logger
	// client will hold the subscription client implementation.
	client TransportClient
	// keepAliveInterval is the actual interval on which the server send keep alive messages to the client.
	keepAliveInterval time.Duration
	// initFunc will check initial payload to see whether to accept the websocket connection.
	initFunc WebsocketInitFunc
}

func NewUniversalProtocolHandler(client TransportClient, protocol Protocol, executorPool ExecutorPool) (*UniversalProtocolHandler, error) {
	return nil, nil
}

// Interface Guards
var _ Engine = (*ExecutorEngine)(nil)
