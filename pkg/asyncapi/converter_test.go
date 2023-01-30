package asyncapi

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/stretchr/testify/require"
)

func TestImportAsyncAPIDocumentString(t *testing.T) {
	versions := []string{"2.0.0", "2.1.0", "2.2.0", "2.3.0", "2.4.0"}
	for _, version := range versions {
		asyncapiDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/streetlights-kafka-%s.yaml", version))
		require.NoError(t, err)
		doc, report := ImportAsyncAPIDocumentString(string(asyncapiDoc))
		if report.HasErrors() {
			t.Fatal(report.Error())
		}
		require.NoError(t, err)
		w := &bytes.Buffer{}
		err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
		require.NoError(t, err)

		graphqlDoc, err := os.ReadFile("./fixtures/streetlights-kafka-2.4.0-and-below.graphql")
		require.NoError(t, err)
		require.Equal(t, string(graphqlDoc), w.String())
	}
}

func TestImportAsyncAPIDocumentString_EmailService(t *testing.T) {
	asyncapiDoc, err := os.ReadFile("./fixtures/email-service-2.0.0.yaml")
	require.NoError(t, err)
	doc, report := ImportAsyncAPIDocumentString(string(asyncapiDoc))
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
	require.NoError(t, err)
	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
	require.NoError(t, err)

	graphqlDoc, err := os.ReadFile("./fixtures/email-service-2.0.0.graphql")
	require.NoError(t, err)
	require.Equal(t, string(graphqlDoc), w.String())
}

func TestImportAsyncAPIDocumentString_PaymentSystemSample(t *testing.T) {
	asyncapiDoc, err := os.ReadFile("./fixtures/payment-system-sample-2.2.0.yaml")
	require.NoError(t, err)
	doc, report := ImportAsyncAPIDocumentString(string(asyncapiDoc))
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
	require.NoError(t, err)
	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
	require.NoError(t, err)

	graphqlDoc, err := os.ReadFile("./fixtures/payment-system-sample-2.2.0.graphql")
	require.NoError(t, err)
	require.Equal(t, string(graphqlDoc), w.String())
}

func TestImportAsyncAPIDocumentString_PaymentSample(t *testing.T) {
	asyncapiDoc, err := os.ReadFile("./fixtures/payment-sample-2.0.0.yaml")
	require.NoError(t, err)
	doc, report := ImportAsyncAPIDocumentString(string(asyncapiDoc))
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
	require.NoError(t, err)
	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
	require.NoError(t, err)

	graphqlDoc, err := os.ReadFile("./fixtures/payment-sample-2.0.0.graphql")
	require.NoError(t, err)
	require.Equal(t, string(graphqlDoc), w.String())
}
