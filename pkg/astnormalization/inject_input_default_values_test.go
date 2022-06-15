package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"testing"
)

const testInputDefaultSchema = `
enum TestEnum {
  ValueOne
  ValueTwo
}

schema {
  mutation: Mutation
}

type Mutation {
  testDefaultValueSimple(data: SimpleTestInput!): String!
  testNestedInputField(data: InputWithNestedField!): String!
}

input SimpleTestInput {
  firstField: String!
  secondField: Int! = 1
}

input InputWithNestedField {
  nested: LowerLevelInput!
}

input LowerLevelInput {
  firstField: Int!
  secondField: TestEnum! = ValueOne
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

	t.Run("nested input field with default values", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation testNestedInputField($a: InputWithNestedField) {
			  testNestedInputField(data: $a)
			}`, "", `
			mutation testNestedInputField($a: InputWithNestedField) {
  				testNestedInputField(data: $a)
			}`, `{"a":{"nested":{}}}`, `{"a":{"nested":{"secondField":"ValueOne"}}}`)
	})
}
