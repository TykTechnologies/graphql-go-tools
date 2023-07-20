package websocket

import (
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
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

	t.Run("should read a message without variables and extensions", func(t *testing.T) {
		data := []byte(`{ "id": "1", "type": "subscribe", "payload": { "operationName": "MyQuery", "query": "query MyQuery { hello }" } }`)
		expectedMessage := &GraphQLTransportWSMessage{
			Id:   "1",
			Type: "subscribe",
			Payload: &GraphQLTransportWSMessagePayload{
				OperationName: "MyQuery",
				Query:         "query MyQuery { hello }",
			},
		}

		reader := GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		}
		message, err := reader.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, message)
	})

	t.Run("should read complete message", func(t *testing.T) {
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
			Payload: &GraphQLTransportWSMessagePayload{
				OperationName: "MyQuery",
				Query:         "query MyQuery($name: String) { hello(name: $name) }",
				Variables:     []byte(`{ "name": "Udo" }`),
				Extensions:    []byte(`{ "Authorization": "Bearer ey123" }`),
			},
		}

		reader := GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		}
		message, err := reader.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, message)
	})
}
