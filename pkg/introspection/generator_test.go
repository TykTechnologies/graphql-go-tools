package introspection

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
)

func TestGenerator_Generate(t *testing.T) {
	starwarsSchemaBytes, err := os.ReadFile("./testdata/starwars.schema.graphql")
	if err != nil {
		panic(err)
	}

	definition, report := astparser.ParseGraphqlDocumentBytes(starwarsSchemaBytes)
	if report.HasErrors() {
		t.Fatal(report)
	}

	gen := NewGenerator()
	var data Data
	gen.Generate(&definition, &report, &data)
	if report.HasErrors() {
		t.Fatal(report)
	}

	outputPretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, "starwars_introspected", outputPretty)
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/starwars_introspected.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("startwars_introspected", fixture, outputPretty)
	}
}

func TestGenerator_Generate_Interfaces_Implementing_Interfaces(t *testing.T) {
	interfacesSchemaBytes, err := os.ReadFile("./testdata/interfaces_implementing_interfaces.graphql")
	if err != nil {
		panic(err)
	}

	definition, report := astparser.ParseGraphqlDocumentBytes(interfacesSchemaBytes)
	if report.HasErrors() {
		t.Fatal(report)
	}

	gen := NewGenerator()
	var data Data
	gen.Generate(&definition, &report, &data)
	if report.HasErrors() {
		t.Fatal(report)
	}

	outputPretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, "interfaces_implementing_interfaces", outputPretty)
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/interfaces_implementing_interfaces.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("interfaces_implements_interfaces", fixture, outputPretty)
	}
}
