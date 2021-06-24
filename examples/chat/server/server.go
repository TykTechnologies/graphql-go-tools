package main

import (
	"log"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"

	"github.com/jensneuse/graphql-go-tools/examples/chat"
)

func main() {
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3080", "http://localhost:3000"},
		AllowCredentials: true,
	})

	srv := handler.New(chat.NewExecutableSchema(chat.New()))

	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	})
	srv.Use(extension.Introspection{})

	http.Handle("/", playground.Handler("Chat", "/query"))
	http.Handle("/query", c.Handler(srv))

	log.Println("Playground running on: http://localhost:8085")
	log.Println("Send operations to: http://localhost:8085/query")
	log.Fatal(http.ListenAndServe(":8085", nil))
}
