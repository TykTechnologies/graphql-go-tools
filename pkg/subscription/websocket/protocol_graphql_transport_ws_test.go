package websocket

import (
	"errors"
	"sync"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
)

func TestGraphQLTransportWSMessageReader_Read(t *testing.T) {
	t.Run("should read a minimal message", func(t *testing.T) {
		data := []byte(`{ "type": "connection_init" }`)
		expectedMessage := &GraphQLTransportWSMessage{
			Type: "connection_init",
		}

		reader := GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		}
		message, err := reader.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, message)
	})

	t.Run("should message with json payload", func(t *testing.T) {
		data := []byte(`{ "id": "1", "type": "connection_init", "payload": { "Authorization": "Bearer ey123" } }`)
		expectedMessage := &GraphQLTransportWSMessage{
			Id:      "1",
			Type:    "connection_init",
			Payload: []byte(`{ "Authorization": "Bearer ey123" }`),
		}

		reader := GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		}
		message, err := reader.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, message)
	})

	t.Run("should read and deserialize subscribe message", func(t *testing.T) {
		data := []byte(`{ 
  "id": "1", 
  "type": "subscribe", 
  "payload": { 
    "operationName": "MyQuery", 
    "query": "query MyQuery($name: String) { hello(name: $name) }", 
    "variables": { "name": "Udo" },
    "extensions": { "Authorization": "Bearer ey123" }
  } 
}`)
		expectedMessage := &GraphQLTransportWSMessage{
			Id:   "1",
			Type: "subscribe",
			Payload: []byte(`{ 
    "operationName": "MyQuery", 
    "query": "query MyQuery($name: String) { hello(name: $name) }", 
    "variables": { "name": "Udo" },
    "extensions": { "Authorization": "Bearer ey123" }
  }`),
		}

		reader := GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		}
		message, err := reader.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, message)

		expectedPayload := &GraphQLTransportWSMessagePayload{
			OperationName: "MyQuery",
			Query:         "query MyQuery($name: String) { hello(name: $name) }",
			Variables:     []byte(`{ "name": "Udo" }`),
			Extensions:    []byte(`{ "Authorization": "Bearer ey123" }`),
		}
		actualPayload, err := reader.DeserializeSubscribePayload(message)
		assert.NoError(t, err)
		assert.Equal(t, expectedPayload, actualPayload)
	})
}

func TestGraphQLTransportWSMessageWriter_WriteConnectionAck(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteConnectionAck()
		assert.Error(t, err)
	})
	t.Run("should successfully write ack message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"connection_ack"}`)
		err := writer.WriteConnectionAck()
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WritePing(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WritePing(nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write ping message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"ping"}`)
		err := writer.WritePing(nil)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should successfully write ping message with payload to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"ping","payload":{"connected_since":"10min"}}`)
		err := writer.WritePing([]byte(`{"connected_since":"10min"}`))
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WritePong(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WritePong(nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write pong message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"pong"}`)
		err := writer.WritePong(nil)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
	t.Run("should successfully write pong message with payload to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"pong","payload":{"connected_since":"10min"}}`)
		err := writer.WritePong([]byte(`{"connected_since":"10min"}`))
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WriteNext(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteNext("1", nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write next message with payload to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"next","payload":{"data":{"hello":"world"}}}`)
		err := writer.WriteNext("1", []byte(`{"data":{"hello":"world"}}`))
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WriteError(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteError("1", nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write error message with payload to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"error","payload":[{"message":"request error"}]}`)
		requestErrors := graphql.RequestErrorsFromError(errors.New("request error"))
		err := writer.WriteError("1", requestErrors)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}

func TestGraphQLTransportWSMessageWriter_WriteComplete(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteComplete("1")
		assert.Error(t, err)
	})
	t.Run("should successfully write complete message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLTransportWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"complete"}`)
		err := writer.WriteComplete("1")
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.readMessageToClient())
	})
}
