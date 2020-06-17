package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type DefinitionValidator struct {
}

func (d *DefinitionValidator) RegisterRule(rule Rule) {

}

func (d *DefinitionValidator) Validate(definition *ast.Document, report *operationreport.Report) ValidationState {
	return Invalid
}
