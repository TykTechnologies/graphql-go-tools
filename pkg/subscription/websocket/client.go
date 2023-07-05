package websocket

import (
	"errors"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"
)

// Client is an actual implementation of the subscription client interface.
type Client struct {
	logger abstractlogger.Logger
	// clientConn holds the actual connection to the client.
	clientConn net.Conn
	// isClosedConnection indicates if the websocket connection is closed.
	isClosedConnection bool
}

// NewClient will create a new websocket subscription client.
func NewClient(logger abstractlogger.Logger, clientConn net.Conn) *Client {
	return &Client{
		logger:     logger,
		clientConn: clientConn,
	}
}

// ReadBytesFromClient will read a subscription message from the websocket client.
func (c *Client) ReadBytesFromClient() ([]byte, error) {
	var data []byte
	var opCode ws.OpCode

	data, opCode, err := wsutil.ReadClientData(c.clientConn)
	if err != nil {
		if c.isClosedConnectionError(err) {
			return nil, nil
		}

		c.logger.Error("websocket.Client.ReadBytesFromClient()",
			abstractlogger.Error(err),
			abstractlogger.ByteString("data", data),
			abstractlogger.Any("opCode", opCode),
		)

		c.isClosedConnectionError(err)

		return nil, err
	}

	return data, nil
}

// WriteBytesToClient will write a subscription message to the websocket client.
func (c *Client) WriteBytesToClient(message []byte) error {
	if c.isClosedConnection {
		return nil
	}

	err := wsutil.WriteServerMessage(c.clientConn, ws.OpText, message)
	if err != nil {
		c.logger.Error("websocket.Client.WriteBytesToClient()",
			abstractlogger.Error(err),
			abstractlogger.ByteString("message", message),
		)

		return err
	}

	return nil
}

// IsConnected will indicate if the websocket conenction is still established.
func (c *Client) IsConnected() bool {
	return !c.isClosedConnection
}

// Disconnect will close the websocket connection.
func (c *Client) Disconnect() error {
	c.logger.Debug("http.GraphQLHTTPRequestHandler.Disconnect()",
		abstractlogger.String("message", "disconnecting client"),
	)
	c.isClosedConnection = true
	return c.clientConn.Close()
}

// isClosedConnectionError will indicate if the given error is a connection closed error.
func (c *Client) isClosedConnectionError(err error) bool {
	if errors.Is(err, wsutil.ClosedError{}) {
		c.isClosedConnection = true
	}

	return c.isClosedConnection
}
