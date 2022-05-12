package astnormalization

import (
	"fmt"
	"testing"
)

func TestExtendUnionType(t *testing.T) {
	t.Run("extend union type by directive", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal @deprecated(reason: "some reason")
					 `, `
					union Mammal @deprecated(reason: "some reason")
					extend union Mammal @deprecated(reason: "some reason")
					`)
	})
	t.Run("extend union type by UnionMemberType", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal = Cat
					 `, `
					union Mammal = Cat
					extend union Mammal = Cat
					`)
	})
	t.Run("extend union type by multiple UnionMemberTypes", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal = Cat | Dog
					 `, `
					union Mammal = Cat | Dog
					extend union Mammal = Cat | Dog
					`)
	})
	t.Run("extend union by multiple directives and union members", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal
					extend union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					 `, `
					union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					extend union Mammal @deprecated(reason: "some reason") @skip(if: false) = Cat | Dog
					`)
	})
	t.Run("extend union type which already has union member", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					union Mammal = Cat
					extend union Mammal @deprecated(reason: "some reason") = Dog
					 `, `
					union Mammal @deprecated(reason: "some reason") = Cat | Dog
					extend union Mammal @deprecated(reason: "some reason") = Dog
					`)
	})
	t.Run("extend non-existent union", func(t *testing.T) {
		run(extendUnionTypeDefinition, testDefinition, `
					extend union Response = SuccessResponse | ErrorResponse
					extend union Mammal @deprecated(reason: "some reason") = Dog
					 `, `
					extend union Response = SuccessResponse | ErrorResponse
					extend union Mammal @deprecated(reason: "some reason") = Dog
					union Response = SuccessResponse | ErrorResponse
					union Mammal @deprecated(reason: "some reason") = Dog
					`)
	})

	t.Run("Extending a union with an existing member returns an error", func(t *testing.T) {
		runAndExpectError(t, extendUnionTypeDefinition, testDefinition, `
			union CatOrDog = Cat | Dog
			extend union CatOrDog = Cat
		`, DuplicateUnionMemberErrorMessage("CatOrDog", "Cat"))
	})
}

func DuplicateUnionMemberErrorMessage(unionName, memberName string) string {
	return fmt.Sprintf("the union named '%s' must have unique members, but the member named '%s' is duplicated", unionName, memberName)
}
