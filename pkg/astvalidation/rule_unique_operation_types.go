package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func UniqueOperationTypes() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

type uniqueOperationTypesVisitor struct {
	*astvisitor.Walker
}
