package websocket

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
)

func TestGraphQLWSMessageReader_Read(t *testing.T) {
	data := []byte(`{ "id": "1", "type": "connection_init", "payload": { "headers": { "key": "value" } } }`)
	expectedMessage := &GraphQLWSMessage{
		Id:      "1",
		Type:    "connection_init",
		Payload: json.RawMessage(`{ "headers": { "key": "value" } }`),
	}

	reader := GraphQLWSMessageReader{
		logger: abstractlogger.Noop{},
	}
	message, err := reader.Read(data)
	assert.NoError(t, err)
	assert.Equal(t, expectedMessage, message)
}

func TestGraphQLWSMessageWriter_WriteData(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteData("1", nil)
		assert.Error(t, err)
	})
	t.Run("should successfully write message data to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"data","payload":{"data":{"hello":"world"}}}`)
		err := writer.WriteData("1", []byte(`{"data":{"hello":"world"}}`))
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.messageToClient)
	})
}

func TestGraphQLWSMessageWriter_WriteComplete(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteComplete("1")
		assert.Error(t, err)
	})
	t.Run("should successfully write complete message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"complete"}`)
		err := writer.WriteComplete("1")
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.messageToClient)
	})
}

func TestGraphQLWSMessageWriter_WriteKeepAlive(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteKeepAlive("1")
		assert.Error(t, err)
	})
	t.Run("should successfully write keep-alive (ka) message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"ka"}`)
		err := writer.WriteKeepAlive("1")
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.messageToClient)
	})
}

func TestGraphQLWSMessageWriter_WriteTerminate(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteTerminate(`failed to accept the websocket connection`)
		assert.Error(t, err)
	})
	t.Run("should successfully write terminate message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"connection_terminate","payload":"failed to accept the websocket connection"}`)
		err := writer.WriteTerminate(`failed to accept the websocket connection`)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.messageToClient)
	})
}

func TestGraphQLWSMessageWriter_WriteConnectionError(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteConnectionError(`could not read message from client`)
		assert.Error(t, err)
	})
	t.Run("should successfully write connection error message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"connection_error","payload":"could not read message from client"}`)
		err := writer.WriteConnectionError(`could not read message from client`)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.messageToClient)
	})
}

func TestGraphQLWSMessageWriter_WriteError(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		requestErrors := graphql.RequestErrorsFromError(errors.New("request error"))
		err := writer.WriteError("1", requestErrors)
		assert.Error(t, err)
	})
	t.Run("should successfully write error message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"id":"1","type":"error","payload":[{"message":"request error"}]}`)
		requestErrors := graphql.RequestErrorsFromError(errors.New("request error"))
		err := writer.WriteError("1", requestErrors)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.messageToClient)
	})
}

func TestGraphQLWSMessageWriter_WriteAck(t *testing.T) {
	t.Run("should return error when error occurs on underlying call", func(t *testing.T) {
		testClient := NewTestClient(true)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		err := writer.WriteAck()
		assert.Error(t, err)
	})
	t.Run("should successfully write ack message to client", func(t *testing.T) {
		testClient := NewTestClient(false)
		writer := GraphQLWSMessageWriter{
			logger: abstractlogger.Noop{},
			client: testClient,
			mu:     &sync.Mutex{},
		}
		expectedMessage := []byte(`{"type":"connection_ack"}`)
		err := writer.WriteAck()
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, testClient.messageToClient)
	})
}
