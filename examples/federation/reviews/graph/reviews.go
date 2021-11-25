package graph

import (
	"fmt"
	"sync"

	"syreclabs.com/go/faker"

	"github.com/jensneuse/graphql-go-tools/examples/federation/reviews/graph/model"
)

var reviews = []*model.Review{
	{
		Body:    "A highly effective form of birth control.",
		Product: &model.Product{Upc: "top-1"},
		Author:  &model.User{ID: "1234"},
	},
	{
		Body:    "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.",
		Product: &model.Product{Upc: "top-2"},
		Author:  &model.User{ID: "1234"},
	},
	{
		Body:    "This is the last straw. Hat you will wear. 11/10",
		Product: &model.Product{Upc: "top-3"},
		Author:  &model.User{ID: "7777"},
	},
}

var m sync.Mutex

var reviewsByUpc = make(map[string]map[string]*model.Review) // map[upc]map[userID]Review
var reviewsByUser = make(map[string]map[string]*model.Review) // map[userID]map[upc]Review

func generateProductReviews(upc string){
	m.Lock()
	defer m.Unlock()

	upcReviews, ok := reviewsByUpc[upc]
	if ok {
		return
	}

	fmt.Println("generating reviews for upc:", upc)

	upcReviews = make(map[string]*model.Review)



	lorem := faker.Lorem()
	reviewItems := make([]*model.Review, 0, generatedReviewsCount)

	for i := 0; i < generatedReviewsCount; i++ {
		userID := fmt.Sprint(i+1)
		review := &model.Review{
			Body:    lorem.Sentence(5),
			Author:  &model.User{ID: userID},
			Product: &model.Product{Upc: upc},
		}

		reviewItems = append(reviewItems, review)
		userReviews, ok := reviewsByUser[userID]
		if !ok {
			userReviews = make(map[string]*model.Review)
		}
		userReviews[upc] = review
		reviewsByUser[userID] = userReviews

		upcReviews[userID] = review
	}

	reviewsByUpc[upc] = upcReviews
}


type sortReviewsByUser []*model.Review
func (x sortReviewsByUser) Len() int           { return len(x) }
func (x sortReviewsByUser) Less(i, j int) bool { return x[i].Author.ID < x[j].Author.ID }
func (x sortReviewsByUser) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

type sortReviewsByUpc []*model.Review
func (x sortReviewsByUpc) Len() int           { return len(x) }
func (x sortReviewsByUpc) Less(i, j int) bool { return x[i].Product.Upc < x[j].Product.Upc }
func (x sortReviewsByUpc) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

