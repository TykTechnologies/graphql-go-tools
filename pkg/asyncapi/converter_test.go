package asyncapi

import (
	"bytes"
	"fmt"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestImportAsyncAPIDocumentByte(t *testing.T) {
	versions := []string{"2.2.0"}
	for _, version := range versions {
		asyncapiDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/streetlights-kafka-%s.yaml", version))
		require.NoError(t, err)
		doc, report := ImportAsyncAPIDocumentByte(asyncapiDoc)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}
		require.NoError(t, err)
		w := &bytes.Buffer{}
		err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
		require.NoError(t, err)
		fmt.Println(w.String())

		graphqlDoc, err := os.ReadFile("./fixtures/streetlights-kafka.graphql")
		require.NoError(t, err)
		require.Equal(t, string(graphqlDoc), w.String())
	}
}