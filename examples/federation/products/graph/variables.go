package graph

import (
	"time"
)

var (
	randomnessEnabled      = true
	itemsGenerationEnabled = false
	minPrice               = 10
	maxPrice               = 1499
	currentPrice           = minPrice
	updateInterval         = time.Second
)
