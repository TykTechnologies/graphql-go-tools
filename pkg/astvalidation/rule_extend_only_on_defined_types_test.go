package astvalidation

import (
	"testing"
)

func TestExtendOnlyOnDefinedTypes(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("scalar types", func(t *testing.T) {
			t.Run("should be valid when scalar type is defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					directive @future on SCALAR
					scalar Date
					extend scalar Date @future
				`, Valid, ExtendOnlyOnDefinedTypes(),
				)
			})

			t.Run("should be invalid when scalar type is not defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					directive @future on SCALAR
					extend scalar Date @future
				`, Invalid, ExtendOnlyOnDefinedTypes(),
				)
			})
		})

		t.Run("object types", func(t *testing.T) {
			t.Run("should be valid when object type is defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					type User {
						id: String
					}

					extend type User {
						email: String
					}
				`, Valid, ExtendOnlyOnDefinedTypes(),
				)
			})

			t.Run("should be invalid when object type is not defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					extend type User {
						email: String
					}
				`, Invalid, ExtendOnlyOnDefinedTypes(),
				)
			})
		})

		t.Run("interface types", func(t *testing.T) {
			t.Run("should be valid when interface type is defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					interface Animal {
						name: String
					}

					extend interface Animal {
						weight: Float
					}
				`, Valid, ExtendOnlyOnDefinedTypes(),
				)
			})

			t.Run("should be invalid when interface type is not defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					extend interface Animal {
						weight: Float
					}
				`, Invalid, ExtendOnlyOnDefinedTypes(),
				)
			})
		})

		t.Run("union types", func(t *testing.T) {
			t.Run("should be valid when union type is defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					union Animal = Cat | Dog
					extend union Animal = Bird
				`, Valid, ExtendOnlyOnDefinedTypes(),
				)
			})

			t.Run("should be invalid when union type is not defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					extend union Animal = Bird
				`, Invalid, ExtendOnlyOnDefinedTypes(),
				)
			})
		})

		t.Run("enum types", func(t *testing.T) {
			t.Run("should be valid when enum type is defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					enum Currency {
						USD
					}

					extend enum Currency {
						Euro
					}
				`, Valid, ExtendOnlyOnDefinedTypes(),
				)
			})

			t.Run("should be invalid when enum type is not defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					extend enum Currency {
						Euro
					}
				`, Invalid, ExtendOnlyOnDefinedTypes(),
				)
			})
		})

		t.Run("input object types", func(t *testing.T) {
			t.Run("should be valid when input object type is defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					input Point {
						x: Int
						y: Int
					}

					extend input Point {
						z: int
					}
				`, Valid, ExtendOnlyOnDefinedTypes(),
				)
			})

			t.Run("should be invalid when input object type is not defined", func(t *testing.T) {
				runDefinitionValidation(t, `
					extend input Point {
						z: int
					}
				`, Invalid, ExtendOnlyOnDefinedTypes(),
				)
			})
		})
	})
}
