package main

import (
	"log"
	"net/http"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/rs/cors"

	"github.com/jensneuse/graphql-go-tools/pkg/testing/subscriptiontesting"
)

func main() {
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3080", "http://localhost:3000"},
		AllowCredentials: true,
	})

	http.Handle("/", playground.Handler("Chat", "/query"))
	http.Handle("/query", c.Handler(subscriptiontesting.GraphQLEndpointHandler()))

	log.Println("Playground running on: http://localhost:8085")
	log.Println("Send operations to: http://localhost:8085/query")
	log.Fatal(http.ListenAndServe(":8085", nil))
}