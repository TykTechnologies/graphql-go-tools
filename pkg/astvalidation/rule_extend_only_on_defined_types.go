package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func ExtendOnlyOnDefinedTypes() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := &knownTypeNamesVisitor{
			Walker: walker,
		}

		walker.RegisterDocumentVisitor(visitor)
	}
}

type extendOnlyOnDefinedTypesVisitor struct {
	*astvisitor.Walker
	definition *ast.Document
}

func (e *extendOnlyOnDefinedTypesVisitor) EnterDocument(operation, definition *ast.Document) {
	e.definition = operation
}
