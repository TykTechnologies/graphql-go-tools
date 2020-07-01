package astvalidation

import (
	"testing"
)

func TestUniqueFieldDefinitionNames(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("no fields", func(t *testing.T) {
			runDefinitionValidation(t, `
					type SomeObject
					interface SomeInterface
					input SomeInputObject
				`, Valid, UniqueFieldDefinitionNames(),
			)
		})

		t.Run("one field", func(t *testing.T) {
			runDefinitionValidation(t, `
					type SomeObject {
						foo: String
					}
					interface SomeInterface {
						foo: String
					}
					input SomeInputObject {
						foo: String
					}
				`, Valid, UniqueFieldDefinitionNames(),
			)
		})

		t.Run("multiple field", func(t *testing.T) {
			runDefinitionValidation(t, `
					type SomeObject {
						foo: String
						bar: String
					}
					interface SomeInterface {
						foo: String
						bar: String
					}
					input SomeInputObject {
						foo: String
						bar: String
					}
				`, Valid, UniqueFieldDefinitionNames(),
			)
		})

		t.Run("extend type with new field", func(t *testing.T) {
			runDefinitionValidation(t, `
					type SomeObject {
						foo: String
					}
					extend type SomeObject {
						bar: String
					}
					extend type SomeObject {
						baz: String
					}
					interface SomeInterface {
						foo: String
					}
					extend interface SomeInterface {
						bar: String
					}
					extend interface SomeInterface {
						baz: String
					}
					input SomeInputObject {
						foo: String
					}
					extend input SomeInputObject {
						bar: String
					}
					extend input SomeInputObject {
						baz: String
					}
				`, Valid, UniqueFieldDefinitionNames(),
			)
		})

		t.Run("duplicate fields inside the same type definition", func(t *testing.T) {
			runDefinitionValidation(t, `
					type SomeObject {
						foo: String
						bar: String
						foo: String
					}
					interface SomeInterface {
						foo: String
						bar: String
						foo: String
					}
					input SomeInputObject {
						foo: String
						bar: String
						foo: String
					}
				`, Invalid, UniqueFieldDefinitionNames(),
			)
		})

		t.Run("extend type with duplicate field", func(t *testing.T) {
			runDefinitionValidation(t, `
					extend type SomeObject {
						foo: String
					}
					type SomeObject {
						foo: String
					}
					extend interface SomeInterface {
						foo: String
					}
					interface SomeInterface {
						foo: String
					}
					extend input SomeInputObject {
						foo: String
					}
					input SomeInputObject {
						foo: String
					}
				`, Invalid, UniqueFieldDefinitionNames(),
			)
		})

		t.Run("duplicate field inside extension", func(t *testing.T) {
			runDefinitionValidation(t, `
					type SomeObject
					extend type SomeObject {
						foo: String
						bar: String
						foo: String
					}
					interface SomeInterface
					extend interface SomeInterface {
						foo: String
						bar: String
						foo: String
					}
					input SomeInputObject
					extend input SomeInputObject {
						foo: String
						bar: String
						foo: String
					}
				`, Invalid, UniqueFieldDefinitionNames(),
			)
		})

		t.Run("duplicate field inside different extensions", func(t *testing.T) {
			runDefinitionValidation(t, `
					type SomeObject
					extend type SomeObject {
						foo: String
					}
					extend type SomeObject {
						foo: String
					}
					interface SomeInterface
					extend interface SomeInterface {
						foo: String
					}
					extend interface SomeInterface {
						foo: String
					}
					input SomeInputObject
					extend input SomeInputObject {
						foo: String
					}
					extend input SomeInputObject {
						foo: String
					}
				`, Invalid, UniqueFieldDefinitionNames(),
			)
		})
	})
}
