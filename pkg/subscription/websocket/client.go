package websocket

import (
	"errors"
	"io"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/subscription"
)

type CloseReason ws.Frame
type CompiledCloseReason []byte

var CompiledCloseReasonNormal CompiledCloseReason = ws.MustCompileFrame(
	ws.NewCloseFrame(ws.NewCloseFrameBody(
		ws.StatusNormalClosure, "Normal Closure",
	)),
)

func NewCloseReason(code uint16, reason string) CloseReason {
	wsCloseFrame := ws.NewCloseFrame(ws.NewCloseFrameBody(
		ws.StatusCode(code), reason,
	))
	return CloseReason(wsCloseFrame)
}

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
	if c.isClosedConnection {
		return nil, subscription.ErrTransportClientClosedConnection
	}

	data, opCode, err := wsutil.ReadClientData(c.clientConn)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.ErrUnexpectedEOF) {
		c.isClosedConnection = true
		return nil, subscription.ErrTransportClientClosedConnection
	} else if err != nil {
		if c.isClosedConnectionError(err) {
			return nil, subscription.ErrTransportClientClosedConnection
		}

		c.logger.Error("websocket.Client.ReadBytesFromClient: after reading from client",
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
		return subscription.ErrTransportClientClosedConnection
	}

	err := wsutil.WriteServerMessage(c.clientConn, ws.OpText, message)
	if errors.Is(err, io.ErrClosedPipe) {
		c.isClosedConnection = true
		return subscription.ErrTransportClientClosedConnection
	} else if err != nil {
		c.logger.Error("websocket.Client.WriteBytesToClient: after writing to client",
			abstractlogger.Error(err),
			abstractlogger.ByteString("message", message),
		)

		return err
	}

	return nil
}

// IsConnected will indicate if the websocket connection is still established.
func (c *Client) IsConnected() bool {
	return !c.isClosedConnection
}

// Disconnect will close the websocket connection.
func (c *Client) Disconnect() error {
	c.logger.Debug("websocket.Client.Disconnect: before disconnect",
		abstractlogger.String("message", "disconnecting client"),
	)
	c.isClosedConnection = true
	return c.clientConn.Close()
}

func (c *Client) DisconnectWithReason(reason interface{}) error {
	var err error
	switch reason.(type) {
	case CloseReason:
		frame := reason.(CloseReason)
		err = c.writeFrame(ws.Frame(frame))
	case CompiledCloseReason:
		compiledReason := reason.(CompiledCloseReason)
		err = c.writeCompiledFrame(compiledReason)
	default:
		c.logger.Error("websocket.Client.DisconnectWithReason: on reason/frame parsing",
			abstractlogger.String("message", "unknown reason provided"),
		)
		frame := NewCloseReason(4400, "unknown reason")
		err = c.writeFrame(ws.Frame(frame))
	}

	c.logger.Debug("websocket.Client.DisconnectWithReason: before sending close frame",
		abstractlogger.String("message", "disconnecting client"),
	)

	if err != nil {
		c.logger.Error("websocket.Client.DisconnectWithReason: after writing close reason",
			abstractlogger.Error(err),
		)
		return err
	}

	return c.Disconnect()
}

func (c *Client) writeFrame(frame ws.Frame) error {
	return ws.WriteFrame(c.clientConn, frame)
}

func (c *Client) writeCompiledFrame(compiledFrame []byte) error {
	_, err := c.clientConn.Write(compiledFrame)
	return err
}

// isClosedConnectionError will indicate if the given error is a connection closed error.
func (c *Client) isClosedConnectionError(err error) bool {
	var closedErr wsutil.ClosedError
	if errors.As(err, &closedErr) {
		c.isClosedConnection = true
	}
	return c.isClosedConnection
}

// Interface Guard
var _ subscription.TransportClient = (*Client)(nil)
