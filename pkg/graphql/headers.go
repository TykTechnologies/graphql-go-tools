package graphql

import (
	"net/http"
)

var ExcludableRuntimeHeaderKeys = []string{
	http.CanonicalHeaderKey("Date"),
	http.CanonicalHeaderKey("Host"),
	http.CanonicalHeaderKey("Sec-WebSocket-Key"),
	http.CanonicalHeaderKey("User-Agent"),
	http.CanonicalHeaderKey("Content-Length"),
}
