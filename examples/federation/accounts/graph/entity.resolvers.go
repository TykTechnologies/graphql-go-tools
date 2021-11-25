package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/jensneuse/graphql-go-tools/examples/federation/accounts/graph/generated"
	"github.com/jensneuse/graphql-go-tools/examples/federation/accounts/graph/model"
)

func (r *entityResolver) FindUserByID(ctx context.Context, id string) (*model.User, error) {
	name := "User " + id
	email := fmt.Sprintf("email-%s@example.com", id)
	if id == "1234" {
		name = "Me"
		email = "me@example.com"
	}

	return &model.User{
		ID:       id,
		Username: name,
		Email:    email,
	}, nil
}

// Entity returns generated.EntityResolver implementation.
func (r *Resolver) Entity() generated.EntityResolver { return &entityResolver{r} }

type entityResolver struct{ *Resolver }
