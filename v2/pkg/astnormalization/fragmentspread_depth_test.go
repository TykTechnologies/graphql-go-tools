package astnormalization

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/asttransform"
)

func TestRealDepthCalculator_CalculateDepthForFragmentSpread(t *testing.T) {
	run := func(t *testing.T, operation, definition, spreadName string, wantDepth int) {
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		def := unsafeparser.ParseGraphqlDocumentString(definition)
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		if err != nil {
			panic(err)
		}

		report := operationreport.Report{}
		calc := FragmentSpreadDepth{}
		var depths Depths
		calc.Get(&op, &def, &report, &depths)
		if report.HasErrors() {
			panic(report.Error())
		}

		gotDepth := -1
		for _, depth := range depths {
			if string(depth.SpreadName) == spreadName {
				gotDepth = depth.Depth
				break
			}
		}

		assert.Equal(t, wantDepth, gotDepth)
	}

	t.Run("simple", func(t *testing.T) {
		run(t, `
				subscription sub {
					...frag1
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					...frag2
				}
				fragment frag2 on Subscription {
					frag2Field
				}`, testDefinition, "frag1", 3)
	})
	t.Run("nested", func(t *testing.T) {
		run(t, `
				subscription sub {
					...frag1
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					...frag2
				}
				fragment frag2 on Subscription {
					frag2Field
				}`, testDefinition, "frag2", 6)
	})
}

// TestRealDepthCalculator_CyclicFragmentSpread guards against a stack overflow
// (TT-17945): fragments that spread each other in a cycle used to make
// calculateNestedDepth/depthForFragment recurse forever instead of failing
// with a normal "fragment cycle" error. FragmentSpreadDepth is currently
// unused by v2's fragment inliner, but it's exported and hardened here too.
func TestRealDepthCalculator_CyclicFragmentSpread(t *testing.T) {
	op := unsafeparser.ParseGraphqlDocumentString(`
				subscription sub {
					newMessage {
						body
					}
				}
				fragment frag1 on Subscription {
					...frag2
				}
				fragment frag2 on Subscription {
					...frag1
				}`)
	def := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	if err != nil {
		t.Fatal(err)
	}

	report := operationreport.Report{}
	calc := FragmentSpreadDepth{}
	var depths Depths

	done := make(chan struct{})
	go func() {
		defer close(done)
		calc.Get(&op, &def, &report, &depths)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("FragmentSpreadDepth.Get did not return - likely an infinite recursion regression")
	}

	assert.True(t, report.HasErrors(), "expected a fragment cycle error to be reported")
	assert.True(t, strings.Contains(report.Error(), "forms fragment cycle"), "expected a fragment cycle error, got: %s", report.Error())
}

func BenchmarkFragmentSpreadDepthCalc_Get(b *testing.B) {
	nested := `
				subscription sub {
					...frag1
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					...frag2
				}
				fragment frag2 on Subscription {
					frag2Field
				}`

	op := unsafeparser.ParseGraphqlDocumentString(nested)
	def := unsafeparser.ParseGraphqlDocumentString(testDefinition)

	calc := &FragmentSpreadDepth{}
	depths := make(Depths, 0, 8)
	report := operationreport.Report{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		depths = depths[:0]
		calc.Get(&op, &def, &report, &depths)
	}
}
