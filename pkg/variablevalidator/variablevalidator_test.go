package variablevalidator

import (
	"github.com/TykTechnologies/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/pkg/asttransform"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

const testDefinition = `
input CustomInput {
    requiredField: String!
    optionalField: String
}

type Query{
    simpleQuery(code: ID): String
}

type Mutation {
    customInputNonNull(in: CustomInput!): String
}`

const (
	testQuery = `
query testQuery($code: ID!){
  simpleQuery(code: $code)
}
`

	customInputMutation = `
mutation testMutation($in: CustomInput!){
	customInputNonNull(in: $in)
}`

	customMultipleOperation = `
query testQuery($code: ID!){
  simpleQuery(code: $code)
}

mutation testMutation($in: CustomInput!){
	customInputNonNull(in: $in)
}
`
)

func TestVariableValidator(t *testing.T) {
	testCases := []struct {
		name          string
		operation     string
		variables     string
		expectedError string
	}{
		{
			name:      "basic variable query",
			operation: testQuery,
			variables: `{"code":"NG"}`,
		},
		{
			name:          "missing variable",
			operation:     testQuery,
			variables:     `{"codes":"NG"}`,
			expectedError: `Required variable "$code" was not provided`,
		},
		{
			name:          "no variable passed",
			operation:     testQuery,
			variables:     "",
			expectedError: `Required variable "$code" was not provided`,
		},
		{
			name:          "nested input variable",
			operation:     customInputMutation,
			variables:     `{"in":{"optionalField":"test"}}`,
			expectedError: `Validation for variable "in" failed: validation failed: /: {"optionalField":"te... "requiredField" value is required`,
		},
		{
			name:      "multiple operation should validate single operation",
			operation: customMultipleOperation,
			variables: `{"code":"NG"}`,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			definitionDocument := unsafeparser.ParseGraphqlDocumentString(testDefinition)
			require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDocument))

			operationDocument := unsafeparser.ParseGraphqlDocumentString(c.operation)

			report := operationreport.Report{}
			validator := NewVariableValidator()
			validator.Validate(&operationDocument, &definitionDocument, nil, []byte(c.variables), &report)

			if c.expectedError == "" && report.HasErrors() {
				t.Fatalf("expected no error, instead got %s", report.Error())
			}
			if c.expectedError != "" {
				require.Equal(t, 1, len(report.ExternalErrors))
				assert.Equal(t, c.expectedError, report.ExternalErrors[0].Message)
			}
		})
	}
}
