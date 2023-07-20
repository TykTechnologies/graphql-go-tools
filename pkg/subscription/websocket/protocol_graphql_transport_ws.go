package websocket

import (
	"encoding/json"

	"github.com/jensneuse/abstractlogger"
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
	Id      string                            `json:"id,omitempty"`
	Type    GraphQLTransportWSMessageType     `json:"type"`
	Payload *GraphQLTransportWSMessagePayload `json:"payload,omitempty"`
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
