package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/TykTechnologies/graphql-go-tools/pkg/testing/federationtesting/products/graph/generated"
	"github.com/TykTechnologies/graphql-go-tools/pkg/testing/federationtesting/products/graph/model"
)

func (r *queryResolver) TopProducts(ctx context.Context, first *int) ([]*model.Product, error) {
	return hats, nil
}

func (r *subscriptionResolver) UpdatedPrice(ctx context.Context) (<-chan *model.Product, error) {
	updatedPrice := make(chan *model.Product)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(updateInterval):
				rand.Seed(time.Now().UnixNano())
				product := hats[0]

				if randomnessEnabled {
					product = hats[rand.Intn(len(hats)-1)]
					product.Price = rand.Intn(maxPrice-minPrice+1) + minPrice
					updatedPrice <- product
					continue
				}

				product.Price = currentPrice
				currentPrice += 1
				updatedPrice <- product
			}
		}
	}()
	return updatedPrice, nil
}

func (r *subscriptionResolver) UpdateProductPrice(ctx context.Context, upc string) (<-chan *model.Product, error) {
	updatedPrice := make(chan *model.Product)
	var product *model.Product

	for _, hat := range hats {
		if hat.Upc == upc {
			product = hat
			break
		}
	}

	if product == nil {
		return nil, fmt.Errorf("unknown product upc: %s", upc)
	}

	go func() {
		var num int

		for {
			num++

			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				product.Price = num
				updatedPrice <- product
			}
		}
	}()

	return updatedPrice, nil
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Subscription returns generated.SubscriptionResolver implementation.
func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }

type queryResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }
