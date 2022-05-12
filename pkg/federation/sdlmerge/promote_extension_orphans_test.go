package sdlmerge

import (
	"testing"
)

func TestPromoteExtensionOrphans(t *testing.T) {
	t.Run("Unique extension orphan is promoted to a type", func(t *testing.T) {
		run(t, newPromoteExtensionOrphansVisitor(), `
			extend union Badges = Boulder
			union Types = Grass | Fire | Water
			extend union Rivals = Gary | RedHairedGuy
		`, `
			union Types = Grass | Fire | Water
			union Badges = Boulder
			union Rivals = Gary | RedHairedGuy
		`)
	})
}
