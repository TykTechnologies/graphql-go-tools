package graph

import (
	"fmt"

	"syreclabs.com/go/faker"

	"github.com/jensneuse/graphql-go-tools/examples/federation/products/graph/model"
)

var hats = []*model.Product{
	{
		Upc:     "top-1",
		Name:    "Trilby",
		Price:   11,
		InStock: 500,
	},
	{
		Upc:     "top-2",
		Name:    "Fedora",
		Price:   22,
		InStock: 1200,
	},
	{
		Upc:     "top-3",
		Name:    "Boater",
		Price:   33,
		InStock: 850,
	},
}

var dynamicHats []*model.Product

func initProducts(amount int) {
	if amount < len(dynamicHats) {
		return
	}

	commerce := faker.Commerce()

	newHats := make([]*model.Product, 0, amount)
	newHats = append(newHats, dynamicHats...)
	startIndex := len(dynamicHats)
	dynamicHats = newHats

	for i := startIndex; i < amount; i++ {
		dynamicHats = append(dynamicHats, &model.Product{
			Upc:     fmt.Sprintf("top-%d", i+1),
			Name:    commerce.ProductName(),
			Price:   int(commerce.Price()),
			InStock: faker.RandomInt(10, 20),
		})
	}
}
