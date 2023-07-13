package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
	"github.com/TykTechnologies/graphql-go-tools/pkg/subscription"
)

const (
	GraphQLWSMessageTypeConnectionInit      = "connection_init"
	GraphQLWSMessageTypeConnectionAck       = "connection_ack"
	GraphQLWSMessageTypeConnectionError     = "connection_error"
	GraphQLWSMessageTypeConnectionTerminate = "connection_terminate"
	GraphQLWSMessageTypeConnectionKeepAlive = "ka"
	GraphQLWSMessageTypeStart               = "start"
	GraphQLWSMessageTypeStop                = "stop"
	GraphQLWSMessageTypeData                = "data"
	GraphQLWSMessageTypeError               = "error"
	GraphQLWSMessageTypeComplete            = "complete"
)

var ErrGraphQLWSUnexpectedMessageType = errors.New("unexpected message type")

type GraphQLWSMessage struct {
	Id      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type GraphQLWSMessageReader struct {
	logger abstractlogger.Logger
}

func (g *GraphQLWSMessageReader) Read(data []byte) (*GraphQLWSMessage, error) {
	var message GraphQLWSMessage
	err := json.Unmarshal(data, &message)
	if err != nil {
		g.logger.Error("websocket.GraphQLWSMessageReader.Read: on json unmarshal",
			abstractlogger.Error(err),
			abstractlogger.ByteString("data", data),
		)

		return nil, err
	}
	return &message, nil
}

type GraphQLWSMessageWriter struct {
	logger abstractlogger.Logger
	client subscription.TransportClient
	mu     *sync.Mutex
}

func (g *GraphQLWSMessageWriter) WriteData(id string, responseData []byte) error {
	message := &GraphQLWSMessage{
		Id:      id,
		Type:    GraphQLWSMessageTypeData,
		Payload: responseData,
	}
	return g.write(message)
}

func (g *GraphQLWSMessageWriter) WriteComplete(id string) error {
	message := &GraphQLWSMessage{
		Id:      id,
		Type:    GraphQLWSMessageTypeComplete,
		Payload: nil,
	}
	return g.write(message)
}

func (g *GraphQLWSMessageWriter) WriteKeepAlive() error {
	message := &GraphQLWSMessage{
		Type:    GraphQLWSMessageTypeConnectionKeepAlive,
		Payload: nil,
	}
	return g.write(message)
}

func (g *GraphQLWSMessageWriter) WriteTerminate(reason string) error {
	payloadBytes, err := json.Marshal(reason)
	if err != nil {
		return err
	}
	message := &GraphQLWSMessage{
		Type:    GraphQLWSMessageTypeConnectionTerminate,
		Payload: payloadBytes,
	}
	return g.write(message)
}

func (g *GraphQLWSMessageWriter) WriteConnectionError(reason string) error {
	payloadBytes, err := json.Marshal(reason)
	if err != nil {
		return err
	}
	message := &GraphQLWSMessage{
		Type:    GraphQLWSMessageTypeConnectionError,
		Payload: payloadBytes,
	}
	return g.write(message)
}

func (g *GraphQLWSMessageWriter) WriteError(id string, errors graphql.RequestErrors) error {
	payloadBytes, err := json.Marshal(errors)
	if err != nil {
		return err
	}
	message := &GraphQLWSMessage{
		Id:      id,
		Type:    GraphQLWSMessageTypeError,
		Payload: payloadBytes,
	}
	return g.write(message)
}

func (g *GraphQLWSMessageWriter) WriteAck() error {
	message := &GraphQLWSMessage{
		Type: GraphQLWSMessageTypeConnectionAck,
	}
	return g.write(message)
}

func (g *GraphQLWSMessageWriter) write(message *GraphQLWSMessage) error {
	jsonData, err := json.Marshal(message)
	if err != nil {
		g.logger.Error("websocket.GraphQLWSMessageWriter.write: on json marshal",
			abstractlogger.Error(err),
			abstractlogger.String("id", message.Id),
			abstractlogger.String("type", message.Type),
			abstractlogger.ByteString("payload", message.Payload),
		)
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.client.WriteBytesToClient(jsonData)
}

type GraphQLWSWriteEventHandler struct {
	logger abstractlogger.Logger
	writer GraphQLWSMessageWriter
}

func (g *GraphQLWSWriteEventHandler) Emit(eventType subscription.EventType, id string, data []byte, err error) {
	messageType := ""
	switch eventType {
	case subscription.EventTypeCompleted:
		messageType = GraphQLWSMessageTypeComplete
	case subscription.EventTypeData:
		messageType = GraphQLWSMessageTypeData
	case subscription.EventTypeError:
		messageType = GraphQLWSMessageTypeError
	case subscription.EventTypeConnectionError:
		messageType = GraphQLWSMessageTypeConnectionError
	default:
		return
	}

	g.HandleWriteEvent(messageType, id, data, err)
}

func (g *GraphQLWSWriteEventHandler) HandleWriteEvent(messageType string, id string, data []byte, providedErr error) {
	var err error
	switch messageType {
	case GraphQLWSMessageTypeComplete:
		err = g.writer.WriteComplete(id)
	case GraphQLWSMessageTypeData:
		err = g.writer.WriteData(id, data)
	case GraphQLWSMessageTypeError:
		err = g.writer.WriteError(id, graphql.RequestErrorsFromError(providedErr))
	case GraphQLWSMessageTypeConnectionError:
		err = g.writer.WriteConnectionError(providedErr.Error())
	case GraphQLWSMessageTypeConnectionKeepAlive:
		err = g.writer.WriteKeepAlive()
	case GraphQLWSMessageTypeConnectionAck:
		err = g.writer.WriteAck()
	default:
		g.logger.Warn("websocket.GraphQLWSWriteEventHandler.Handle: on write event handling with unexpected message type",
			abstractlogger.Error(err),
			abstractlogger.String("id", id),
			abstractlogger.String("type", messageType),
			abstractlogger.ByteString("payload", data),
			abstractlogger.Error(providedErr),
		)
		return
	}
	if err != nil {
		g.logger.Error("websocket.GraphQLWSWriteEventHandler.Handle: on write event handling",
			abstractlogger.Error(err),
			abstractlogger.String("id", id),
			abstractlogger.String("type", messageType),
			abstractlogger.ByteString("payload", data),
			abstractlogger.Error(providedErr),
		)
	}
}

type ProtocolGraphQLWSHandler struct {
	logger            abstractlogger.Logger
	reader            GraphQLWSMessageReader
	writeEventHandler GraphQLWSWriteEventHandler
	keepAliveInterval time.Duration
	initFunc          InitFunc
}

func (p *ProtocolGraphQLWSHandler) Handle(ctx context.Context, engine subscription.Engine, data []byte) error {
	message, err := p.reader.Read(data)
	if err != nil {
		p.logger.Error("websocket.ProtocolGraphQLWSHandler.Handle: on message reading",
			abstractlogger.Error(err),
			abstractlogger.ByteString("payload", data),
		)
	}

	switch message.Type {
	case GraphQLWSMessageTypeConnectionInit:
		ctx, err = p.handleInit(ctx, message.Payload)
		if err != nil {
			p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionError, "", nil, errors.New("failed to accept the websocket connection"))
			return engine.TerminateAllConnections(&p.writeEventHandler)
		}

		go p.handleKeepAlive(ctx)
	case GraphQLWSMessageTypeStart:
		return engine.StartOperation(ctx, message.Id, message.Payload, &p.writeEventHandler)
	case GraphQLWSMessageTypeStop:
		return engine.StopSubscription(message.Id, &p.writeEventHandler)
	case GraphQLWSMessageTypeConnectionTerminate:
		return engine.TerminateAllConnections(&p.writeEventHandler)
	default:
		p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionError, message.Id, nil, fmt.Errorf("%s: %s", ErrGraphQLWSUnexpectedMessageType.Error(), message.Type))
	}

	return nil
}

func (p *ProtocolGraphQLWSHandler) EventHandler() subscription.EventHandler {
	return &p.writeEventHandler
}

func (p *ProtocolGraphQLWSHandler) handleInit(ctx context.Context, payload []byte) (context.Context, error) {
	initCtx := ctx
	if p.initFunc != nil && len(payload) > 0 {
		var initPayload InitPayload
		initPayload = payload

		// check initial payload to see whether to accept the websocket connection
		var err error
		if initCtx, err = p.initFunc(ctx, initPayload); err != nil {
			return initCtx, err
		}
	}

	p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionAck, "", nil, nil)
	return initCtx, nil
}

func (p *ProtocolGraphQLWSHandler) handleKeepAlive(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.keepAliveInterval):
			p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionKeepAlive, "", nil, nil)
		}
	}
}
