package astvalidation

import (
	"testing"
)

func TestUniqueOperationTypes(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("no schema definition", func(t *testing.T) {
			runDefinitionValidation(t, `
					type Foo
				`, Valid, UniqueOperationTypes(),
			)
		})
	})
}
