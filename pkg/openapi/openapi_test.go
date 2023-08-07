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

 // Content of fixtures/v3.0.0/example_oas7.graphql
 schema {
     query: Query
     mutation: Mutation
 }
 
 type Query {
     "Find a device by name."
     findDeviceByName(deviceName: String!): Device
     "Return a device collection."
     findDevices: [Device]
     "Return a user."
     user: User
 }
 
 type Mutation {
     "Create and return a device."
     createDevice(deviceInput: DeviceInput!): Device
     "Replace a device by name."
     replaceDeviceByName(deviceInput: DeviceInput!, deviceName: String!): Device
 }
 
 "A device is an object connected to the network"
 type Device {
     "The device name in the network"
     name: String!
     status: Boolean
     "The device owner Name"
     userName: String!
 }
 
 input DeviceInput {
     name: String!
     status: Boolean
     userName: String!
 }
 
 "A user represents a natural person"
 type User {
     "The legal name of a user"
     name: String
 }