// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

type Brand interface {
	IsBrand()
}

type MetadataOrError interface {
	IsMetadataOrError()
}

type Product interface {
	IsProduct()
}

type ProductDetails interface {
	IsProductDetails()
}

type Thing interface {
	IsThing()
}

type Vehicle interface {
	IsVehicle()
}

type Amazon struct {
	Referrer *string `json:"referrer"`
}

func (Amazon) IsBrand() {}

type Car struct {
	ID          string  `json:"id"`
	Description *string `json:"description"`
	Price       *string `json:"price"`
}

func (Car) IsVehicle() {}
func (Car) IsThing()   {}
func (Car) IsEntity()  {}

type Error struct {
	Code    *int    `json:"code"`
	Message *string `json:"message"`
}

func (Error) IsMetadataOrError() {}

type Furniture struct {
	Upc      string                   `json:"upc"`
	Sku      string                   `json:"sku"`
	Name     *string                  `json:"name"`
	Price    *string                  `json:"price"`
	Brand    Brand                    `json:"brand"`
	Metadata []MetadataOrError        `json:"metadata"`
	Details  *ProductDetailsFurniture `json:"details"`
	InStock  int                      `json:"inStock"`
}

func (Furniture) IsProduct() {}
func (Furniture) IsEntity()  {}

type Ikea struct {
	Asile *int `json:"asile"`
}

func (Ikea) IsBrand() {}
func (Ikea) IsThing() {}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (KeyValue) IsMetadataOrError() {}

type ProductDetailsBook struct {
	Country *string `json:"country"`
	Pages   *int    `json:"pages"`
}

func (ProductDetailsBook) IsProductDetails() {}

type ProductDetailsFurniture struct {
	Country *string `json:"country"`
	Color   *string `json:"color"`
}

func (ProductDetailsFurniture) IsProductDetails() {}

type User struct {
	ID      string  `json:"id"`
	Vehicle Vehicle `json:"vehicle"`
	Thing   Thing   `json:"thing"`
}

func (User) IsEntity() {}

type Van struct {
	ID          string  `json:"id"`
	Description *string `json:"description"`
	Price       *string `json:"price"`
}

func (Van) IsVehicle() {}
func (Van) IsEntity()  {}
