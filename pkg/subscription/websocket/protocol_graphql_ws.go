package websocket

import (
	"encoding/json"
	"sync"

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

func (g *GraphQLWSMessageWriter) WriteKeepAlive(id string) error {
	message := &GraphQLWSMessage{
		Id:      id,
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

type ProtocolGraphQLWSHandler struct {
	logger abstractlogger.Logger
	reader GraphQLWSMessageReader
	writer GraphQLWSMessageWriter
}
