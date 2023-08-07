package openapi

import (
	"bytes"
	"fmt"
	"os"
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
 }