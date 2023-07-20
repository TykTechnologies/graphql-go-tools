package websocket

import (
	"encoding/json"
	"sync"

	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
	"github.com/TykTechnologies/graphql-go-tools/pkg/subscription"
)

type GraphQLTransportWSMessageType string

const (
	GraphQLTransportWSMessageTypeConnectionInit GraphQLTransportWSMessageType = "connection_init"
	GraphQLTransportWSMessageTypeConnectionAck  GraphQLTransportWSMessageType = "connection_ack"
	GraphQLTransportWSMessageTypePing           GraphQLTransportWSMessageType = "ping"
	GraphQLTransportWSMessageTypePong           GraphQLTransportWSMessageType = "pong"
	GraphQLTransportWSMessageTypeSubscribe      GraphQLTransportWSMessageType = "subscribe"
	GraphQLTransportWSMessageTypeNext           GraphQLTransportWSMessageType = "next"
	GraphQLTransportWSMessageTypeError          GraphQLTransportWSMessageType = "error"
	GraphQLTransportWSMessageTypeComplete       GraphQLTransportWSMessageType = "complete"
)

type GraphQLTransportWSMessage struct {
	Id      string                        `json:"id,omitempty"`
	Type    GraphQLTransportWSMessageType `json:"type"`
	Payload json.RawMessage               `json:"payload,omitempty"`
}

type GraphQLTransportWSMessagePayload struct {
	OperationName string          `json:"operationName,omitempty"`
	Query         string          `json:"query"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Extensions    json.RawMessage `json:"extensions,omitempty"`
}

type GraphQLTransportWSMessageReader struct {
	logger abstractlogger.Logger
}

// Read deserializes a byte slice to the GraphQLTransportWSMessage struct.
func (g *GraphQLTransportWSMessageReader) Read(data []byte) (*GraphQLTransportWSMessage, error) {
	var message GraphQLTransportWSMessage
	err := json.Unmarshal(data, &message)
	if err != nil {
		g.logger.Error("websocket.GraphQLTransportWSMessageReader.Read: on json unmarshal",
			abstractlogger.Error(err),
			abstractlogger.ByteString("data", data),
		)

		return nil, err
	}
	return &message, nil
}

func (g *GraphQLTransportWSMessageReader) DeserializeSubscribePayload(message *GraphQLTransportWSMessage) (*GraphQLTransportWSMessagePayload, error) {
	var deserializedPayload GraphQLTransportWSMessagePayload
	err := json.Unmarshal(message.Payload, &deserializedPayload)
	if err != nil {
		g.logger.Error("websocket.GraphQLTransportWSMessageReader.DeserializeSubscribePayload: on subscribe payload deserialization",
			abstractlogger.Error(err),
			abstractlogger.ByteString("payload", message.Payload),
		)
		return nil, err
	}

	return &deserializedPayload, nil
}

// GraphQLTransportWSMessageWriter can be used to write graphql-transport-ws messages to a transport client.
type GraphQLTransportWSMessageWriter struct {
	logger abstractlogger.Logger
	client subscription.TransportClient
	mu     *sync.Mutex
}

// WriteConnectionAck writes a message of type 'connection_ack' to the transport client.
func (g *GraphQLTransportWSMessageWriter) WriteConnectionAck() error {
	message := &GraphQLTransportWSMessage{
		Type: GraphQLTransportWSMessageTypeConnectionAck,
	}
	return g.write(message)
}

// WritePing writes a message of type 'ping' to the transport client. Payload is optional.
func (g *GraphQLTransportWSMessageWriter) WritePing(payload []byte) error {
	message := &GraphQLTransportWSMessage{
		Type:    GraphQLTransportWSMessageTypePing,
		Payload: payload,
	}
	return g.write(message)
}

// WritePong writes a message of type 'pong' to the transport client. Payload is optional.
func (g *GraphQLTransportWSMessageWriter) WritePong(payload []byte) error {
	message := &GraphQLTransportWSMessage{
		Type:    GraphQLTransportWSMessageTypePong,
		Payload: payload,
	}
	return g.write(message)
}

// WriteNext writes a message of type 'next' to the transport client including the execution result as payload.
func (g *GraphQLTransportWSMessageWriter) WriteNext(id string, executionResult []byte) error {
	message := &GraphQLTransportWSMessage{
		Id:      id,
		Type:    GraphQLTransportWSMessageTypeNext,
		Payload: executionResult,
	}
	return g.write(message)
}

// WriteError writes a message of type 'error' to the transport client including the graphql errors as payload.
func (g *GraphQLTransportWSMessageWriter) WriteError(id string, graphqlErrors graphql.RequestErrors) error {
	payloadBytes, err := json.Marshal(graphqlErrors)
	if err != nil {
		return err
	}
	message := &GraphQLTransportWSMessage{
		Id:      id,
		Type:    GraphQLTransportWSMessageTypeError,
		Payload: payloadBytes,
	}
	return g.write(message)
}

// WriteComplete writes a message of type 'complete' to the transport client.
func (g *GraphQLTransportWSMessageWriter) WriteComplete(id string) error {
	message := &GraphQLTransportWSMessage{
		Id:   id,
		Type: GraphQLTransportWSMessageTypeComplete,
	}
	return g.write(message)
}

func (g *GraphQLTransportWSMessageWriter) write(message *GraphQLTransportWSMessage) error {
	jsonData, err := json.Marshal(message)
	if err != nil {
		g.logger.Error("websocket.GraphQLTransportWSMessageWriter.write: on json marshal",
			abstractlogger.Error(err),
			abstractlogger.String("id", message.Id),
			abstractlogger.String("type", string(message.Type)),
			abstractlogger.Any("payload", message.Payload),
		)
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.client.WriteBytesToClient(jsonData)
}
