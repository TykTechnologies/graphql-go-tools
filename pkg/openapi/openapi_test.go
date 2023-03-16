package openapi

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/stretchr/testify/require"
)

func testFixtureFile(t *testing.T, version, name string) {
	asyncapiDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/%s/%s", version, name))
	require.NoError(t, err)

	doc, report := ImportOpenAPIDocumentString(string(asyncapiDoc))
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
	require.NoError(t, err)
	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
	require.NoError(t, err)

	name = strings.Trim(strings.Trim(name, ".yaml"), ".json")
	graphqlDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/%s/%s.graphql", version, name))
	require.NoError(t, err)
	require.Equal(t, string(graphqlDoc), w.String())
}

func TestOpenAPI_v3_0_0(t *testing.T) {
	t.Run("petstore-expanded.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "petstore-expanded.yaml")
	})

	t.Run("petstore.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "petstore.yaml")
	})

	t.Run("example_oas7.json", func(t *testing.T) {
		// Source: https://github.com/IBM/openapi-to-graphql/blob/master/packages/openapi-to-graphql/test/fixtures/example_oas7.json
		testFixtureFile(t, "v3.0.0", "example_oas7.json")
	})

}
