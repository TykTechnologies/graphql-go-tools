package graph

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/debug"

	"github.com/jensneuse/graphql-go-tools/examples/federation/reviews/graph/generated"
)

type EndpointOptions struct {
	EnableDebug           bool
	EnableItemsGeneration bool
	GeneratedReviewsCount *int
}

var TestOptions = EndpointOptions{
	EnableDebug: false,
}

func GraphQLEndpointHandler(opts EndpointOptions) http.Handler {
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &Resolver{}}))
	if opts.EnableDebug {
		srv.Use(&debug.Tracer{})
	}

	itemsGenerationEnabled = opts.EnableItemsGeneration
	if opts.GeneratedReviewsCount != nil {
		generatedReviewsCount = *opts.GeneratedReviewsCount
	}

	return srv
}
