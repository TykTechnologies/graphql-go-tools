package astvalidation

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astparser"
	"github.com/TykTechnologies/graphql-go-tools/pkg/asttransform"
)

func TestMain(m *testing.M) {
	printMemUsage := func(out io.Writer) {
		bToMb := func(b uint64) uint64 {
			return b / 1024 / 1024
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		// For info on each, see: https://golang.org/pkg/runtime/#MemStats
		fmt.Fprintf(out, "Alloc = %v MiB", bToMb(m.Alloc))
		fmt.Fprintf(out, "\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
		fmt.Fprintf(out, "\tSys = %v MiB", bToMb(m.Sys))
		fmt.Fprintf(out, "\tNumGC = %v\n", m.NumGC)
	}

	printMemUsage(os.Stderr)
	exitCode := m.Run()
	printMemUsage(os.Stderr)
	os.Exit(exitCode)
}

func runDefinitionValidation(t testing.TB, definitionInput string, expectation ValidationState, rules ...Rule) {
	definition, report := astparser.ParseGraphqlDocumentString(definitionInput)
	require.False(t, report.HasErrors())

	err := asttransform.MergeDefinitionWithBaseSchema(&definition)
	require.NoError(t, err)

	validator := &DefinitionValidator{}
	for _, rule := range rules {
		validator.RegisterRule(rule)
	}

	result := validator.Validate(&definition, &report)
	assert.Equal(t, expectation, result)
}
