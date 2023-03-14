package openapi

import (
	"bytes"
	"os"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIv3(t *testing.T) {
	input, err := os.ReadFile("./fixtures/v3.0.0/petstore-expanded.yaml")
	require.NoError(t, err)

	doc, report := ImportOpenAPIDocumentByte(input)
	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
	require.NoError(t, err)

	graphqlDoc, err := os.ReadFile("./fixtures/v3.0.0/petstore-expanded.graphql")
	require.NoError(t, err)
	require.Equal(t, string(graphqlDoc), w.String())
}
