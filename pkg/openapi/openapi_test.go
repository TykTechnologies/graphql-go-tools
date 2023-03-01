package openapi

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIv3(t *testing.T) {
	input, err := os.ReadFile("./fixtures/v3.0.0/petstore-expanded.yaml")
	require.NoError(t, err)

	doc, err := ImportOpenAPIDocumentByte(input)
	fmt.Println(err)

	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
	require.NoError(t, err)
	fmt.Println(w.String())
}
