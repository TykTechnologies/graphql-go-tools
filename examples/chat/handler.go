package chat

import (
	"fmt"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/gorilla/websocket"
)

func GraphQLEndpointHandler() http.Handler {
	srv := handler.New(NewExecutableSchema(New()))

	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				fmt.Printf("-> new incoming request with headers: %v\n", r.Header)
				return true
			},
		},
	})
	srv.Use(extension.Introspection{})

	return srv
}
