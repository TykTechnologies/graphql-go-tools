//go:generate go run -mod=mod github.com/99designs/gqlgen
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/playground"

	"github.com/jensneuse/graphql-go-tools/examples/federation/products/graph"
)

const defaultPort = "4002"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	endpointOpts := graph.EndpointOptions{EnableRandomness: true}

	if os.Getenv("ITEM_GENERATION") == "1" {
		endpointOpts.EnableItemsGeneration = true
	}

	if os.Getenv("DEBUG") != "" {
		endpointOpts.EnableDebug = true
	}

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", graph.GraphQLEndpointHandler(endpointOpts))
	http.HandleFunc("/websocket_connections", graph.WebsocketConnectionsHandler)

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
