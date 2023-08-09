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
   		// Updated expected output to include the description for the `Device` type
   		// Expected output: "schema {\n    query: Query\n    mutation: Mutation\n}\n\ntype Query {\n    \"Find a device by name.\"\n    findDeviceByName(deviceName: String!): Device\n    \"Return a device collection.\"\n    findDevices: [Device]\n    \"Return a user.\"\n    user: User\n}\n\ntype Mutation {\n    \"Create and return a device.\"\n    createDevice(deviceInput: DeviceInput!): Device\n    \"Replace a device by name.\"\n    replaceDeviceByName(deviceInput: DeviceInput!, deviceName: String!): Device\n}\n\n\"A device is an object connected to the network\"\ntype Device {\n    \"The device name in the network\"\n    name: String!\n    status: Boolean\n    \"The device owner Name\"\n    userName: String!\n}\n\ninput DeviceInput {\n    name: String!\n    status: Boolean\n    userName: String!\n}\n\n\"A user represents a natural person\"\ntype User {\n    \"The legal name of a user\"\n    name: String\n}"
   		testFixtureFile(t, "v3.0.0", "example_oas7.json")
   	})

	t.Run("EmployeesApiBasic.yaml", func(t *testing.T) {
		// Source https://github.com/zosconnect/test-samples/blob/main/oas/EmployeesApiBasic.yaml
		testFixtureFile(t, "v3.0.0", "EmployeesApiBasic.yaml")
	})

	t.Run("EmployeesApi.yaml", func(t *testing.T) {
		// Source https://github.com/zosconnect/test-samples/blob/main/oas/EmployeesApiBasic.yaml
		testFixtureFile(t, "v3.0.0", "EmployeesApi.yaml")
	})

	t.Run("example_oas3.json", func(t *testing.T) {
		// Source: https://github.com/IBM/openapi-to-graphql/blob/master/packages/openapi-to-graphql/test/fixtures/example_oas3.json
		testFixtureFile(t, "v3.0.0", "example_oas3.json")
	})

}