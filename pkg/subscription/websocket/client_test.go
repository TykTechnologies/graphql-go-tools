package websocket

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_WriteToClient(t *testing.T) {
	connToServer, connToClient := net.Pipe()

	websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should write successfully to client", func(t *testing.T) {
		messageToClient := []byte(`{
			"id": "1",
			"type": "data",
			"payload": {"data":null}
		}`)

		go func() {
			err := websocketClient.WriteBytesToClient(messageToClient)
			assert.NoError(t, err)
		}()

		data, opCode, err := wsutil.ReadServerData(connToServer)
		require.NoError(t, err)
		require.Equal(t, ws.OpText, opCode)

		time.Sleep(10 * time.Millisecond)
		assert.Equal(t, messageToClient, data)
	})

	t.Run("should not write to client when connection is closed", func(t *testing.T) {
		err := connToServer.Close()
		require.NoError(t, err)

		websocketClient.isClosedConnection = true

		err = websocketClient.WriteBytesToClient([]byte(""))
		assert.NoError(t, err)
	})
}

func TestClient_ReadFromClient(t *testing.T) {
	t.Run("should successfully read from client", func(t *testing.T) {
		connToServer, connToClient := net.Pipe()
		websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

		messageToServer := []byte(`{
			"id": "1",
			"type": "data",
			"payload": {"data":null}
		}`)

		go func() {
			err := wsutil.WriteClientText(connToServer, messageToServer)
			require.NoError(t, err)
		}()

		time.Sleep(10 * time.Millisecond)

		messageFromClient, err := websocketClient.ReadBytesFromClient()
		assert.NoError(t, err)
		assert.Equal(t, messageToServer, messageFromClient)
	})
}

func TestClient_IsConnected(t *testing.T) {
	_, connToClient := net.Pipe()
	websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should return true when a connection is established", func(t *testing.T) {
		isConnected := websocketClient.IsConnected()
		assert.True(t, isConnected)
	})

	t.Run("should return false when a connection is closed", func(t *testing.T) {
		err := connToClient.Close()
		require.NoError(t, err)

		websocketClient.isClosedConnection = true

		isConnected := websocketClient.IsConnected()
		assert.False(t, isConnected)
	})
}

func TestClient_Disconnect(t *testing.T) {
	_, connToClient := net.Pipe()
	websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should disconnect and indicate a closed connection", func(t *testing.T) {
		err := websocketClient.Disconnect()
		assert.NoError(t, err)
		assert.Equal(t, true, websocketClient.isClosedConnection)
	})
}

func TestClient_isClosedConnectionError(t *testing.T) {
	_, connToClient := net.Pipe()
	websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should not close connection when it is not a closed connection error", func(t *testing.T) {
		isClosedConnectionError := websocketClient.isClosedConnectionError(errors.New("no closed connection err"))
		assert.False(t, isClosedConnectionError)
	})

	t.Run("should close connection when it is a closed connection error", func(t *testing.T) {
		isClosedConnectionError := websocketClient.isClosedConnectionError(wsutil.ClosedError{})
		assert.True(t, isClosedConnectionError)
	})
}

type TestClient struct {
	mu              *sync.Mutex
	messageToClient []byte
	shouldFail      bool
}

func NewTestClient(shouldFail bool) *TestClient {
	return &TestClient{
		mu:              &sync.Mutex{},
		messageToClient: nil,
		shouldFail:      shouldFail,
	}
}

func (t *TestClient) ReadBytesFromClient() ([]byte, error) {
	return nil, nil
}

func (t *TestClient) WriteBytesToClient(message []byte) error {
	if t.shouldFail {
		return errors.New("shouldFail is true")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageToClient = message
	return nil
}

func (t *TestClient) IsConnected() bool {
	return false
}

func (t *TestClient) Disconnect() error {
	return nil
}
