package sdlmerge

import (
	"fmt"
	"testing"
)

func TestExtendUnionType(t *testing.T) {
	t.Run("extend union types", func(t *testing.T) {
		run(t, newExtendUnionTypeDefinition(), `
			type Dog {
				name: String
			}

			union Animal = Dog
			
			type Cat {
				name: String
			}

			type Bird {
				name: String
			}

			extend union Animal = Bird | Cat
		`, `
			type Dog {
				name: String
			}

			union Animal = Dog | Bird | Cat
			
			type Cat {
				name: String
			}

			type Bird {
				name: String
			}

			extend union Animal = Bird | Cat
		`)
	})

	// When federating, duplicate value types must be identical or the federation will fail.
	// Consequently, when extending, all duplicate value types should also be extended.
	t.Run("Duplicate unions are each extended", func(t *testing.T) {
		runAndExpectError(t, newExtendUnionTypeDefinition(), `
			type Dog {
				name: String
			}

			union Animal = Dog
			
			type Cat {
				name: String
			}

			type Bird {
				name: String
			}

			union Animal = Dog

			extend union Animal = Bird | Cat
		`, SharedTypeExtensionErrorMessage("Animal"))
	})

	t.Run("Extending a union with an existing member returns an error", func(t *testing.T) {
		runAndExpectError(t, newExtendUnionTypeDefinition(), `
			union Animal = Dog | Cat
	
			extend union Animal = Dog
		`, DuplicateUnionMemberErrorMessage("Animal", "Dog"))
	})
}

func DuplicateUnionMemberErrorMessage(unionName, memberName string) string {
	return fmt.Sprintf("the union named '%s' must have unique members, but the member named '%s' is duplicated", unionName, memberName)
}
