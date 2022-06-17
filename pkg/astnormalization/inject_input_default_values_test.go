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
  mutationExtractDefaultVariable(in: PassedWithDefault = {firstField: "test"}): String
  mutationNestedMissing(in: InputWithDefaultFieldsNested): String
  
}

input InputWithDefaultFieldsNested {
    first: String!
    nested: LowerLevelInput = {firstField: 0}
}

input SimpleTestInput {
  firstField: String! = "firstField"
  secondField: Int! = 1
  thirdField: Int!
}

input PassedWithDefault {
    firstField: String!
    second: Int! = 0
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

	t.Run("multiple variables for operation", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation combinedMutation($a: SimpleTestInput, $b: InputWithNestedField) {
  				testDefaultValueSimple(data: $a)
  				testNestedInputField(data: $b)
			}`, "", `
			mutation combinedMutation($a: SimpleTestInput, $b: InputWithNestedField) {
  				testDefaultValueSimple(data: $a)
  				testNestedInputField(data: $b)
			}`, `{"b":{"nested":{}},"a":{"firstField":"test"}}`,
			`{"b":{"nested":{"secondField":"ValueOne"}},"a":{"firstField":"test","secondField":1}}`,
		)
	})

	t.Run("default field object omitting nested default field", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation($a: InputWithDefaultFieldsNested){
				mutationNestedMissing(in: $a)
			}`, "", `
			mutation($a: InputWithDefaultFieldsNested){
				mutationNestedMissing(in: $a)
			}`, `{"a":{"first":"test"}}`, `{"a":{"first":"test","nested":{"firstField":0,"secondField":"ValueOne"}}}`)
	})

	t.Run("run with extract variables", func(t *testing.T) {
		runWithVariables(t, extractVariables, testInputDefaultSchema, `
		mutation {
  			testNestedInputField(data: { nested: { firstField: 1 } })
		}`, "", `
		mutation($a: InputWithNestedField!) {
  				testNestedInputField(data: $a)
		}`, "", `{"a":{"nested":{"firstField":1,"secondField":"ValueOne"}}}`, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		})
	})

	t.Run("run with variable default value", func(t *testing.T) {
		runWithVariablesDefaultValues(t, extractVariablesDefaultValue, testInputDefaultSchema, `
			mutation {
				mutationExtractDefaultVariable()
			}`, "", `
			mutation($a: PassedWithDefault) {
    			mutationExtractDefaultVariable(in: $a)
			}`, "", `{"a":{"firstField":"test","second":0}}`, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		})

	})
}
