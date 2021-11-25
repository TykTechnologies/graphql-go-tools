//go:generate go run -mod=mod github.com/99designs/gqlgen
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/99designs/gqlgen/graphql/playground"

	"github.com/jensneuse/graphql-go-tools/examples/federation/reviews/graph"
)

const defaultPort = "4003"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	endpointOpts := graph.EndpointOptions{}
	if os.Getenv("DEBUG") != "" {
		endpointOpts.EnableDebug = true
	}

	if os.Getenv("ITEM_GENERATION") == "1" {
		endpointOpts.EnableItemsGeneration = true
	}

	if count := os.Getenv("REVIEWS_COUNT"); count != "" {
		itemCount, err := strconv.Atoi(count)
		if err == nil {
			endpointOpts.GeneratedReviewsCount = &itemCount
		}
	}

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", graph.GraphQLEndpointHandler(endpointOpts))

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
