package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"testing"
)

const testInputDefaultSchema = `
schema {
    mutation: Mutation
}

type Mutation {
    testDefaultValueSimple(data: SimpleTestInput!): String!
}

input SimpleTestInput {
    firstField: String!
    secondField: Int! = 1
}
`

func TestInputDefaultValueExtraction(t *testing.T) {
	t.Run("simple default value extract", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation testDefaultValueSimple($a: SimpleTestInput!) {
  				testDefaultValueSimple(data: $a)
			}`, "", `
			mutation testDefaultValueSimple($a: SimpleTestInput!) {
  				testDefaultValueSimple(data: $a)
			}`, `{"a":{"firstField":"test"}}`, `{"a":{"firstField":"test","secondField":1}}`)
	})
}
