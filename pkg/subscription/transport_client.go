package subscription

// TransportClient provides an interface that can be implemented by any possible subscription client like websockets, mqtt, etc.
// It operates with raw byte slices.
type TransportClient interface {
	// ReadBytesFromClient will invoke a read operation from the client connection and return a byte slice.
	ReadBytesFromClient() ([]byte, error)
	// WriteBytesToClient will invoke a write operation to the client connection using a byte slice.
	WriteBytesToClient([]byte) error
	// IsConnected will indicate if a connection is still established.
	IsConnected() bool
	// Disconnect will close the connection between server and client.
	Disconnect() error
}
