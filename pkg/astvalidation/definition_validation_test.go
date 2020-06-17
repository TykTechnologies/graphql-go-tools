package astvalidation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
)

func runDefinitionValidation(t *testing.T, definitionInput string, expectation ValidationState, rules ...Rule) {
	definition, report := astparser.ParseGraphqlDocumentString(definitionInput)
	require.False(t, report.HasErrors())

	validator := &DefinitionValidator{}
	for _, rule := range rules {
		validator.RegisterRule(rule)
	}

	result := validator.Validate(&definition, &report)
}
