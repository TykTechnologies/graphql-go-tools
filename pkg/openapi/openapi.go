package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/pkg/lexer/literal"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/getkin/kin-openapi/openapi3"
	"sort"
	"strings"
)

type converter struct {
	openapi        *openapi3.T
	knownFullTypes map[string]struct{}
	fullTypes      []introspection.FullType
}

// __TypeKind of introspection is an unexported type. In order to overcome the problem,
// this function creates and returns a TypeRef for a given kind. kind is a AsyncAPI type.
func getTypeRef(kind string) (introspection.TypeRef, error) {
	// See introspection_enum.go
	switch kind {
	case "string", "integer", "number", "boolean":
		return introspection.TypeRef{Kind: 0}, nil
	case "object":
		return introspection.TypeRef{Kind: 3}, nil
	case "array":
		return introspection.TypeRef{Kind: 1}, nil
	}
	return introspection.TypeRef{}, errors.New("unknown type")
}

func asyncAPITypeToGQLType(asyncAPIType string) (string, error) {
	// See https://www.asyncapi.com/docs/reference/specification/v2.4.0#dataTypeFormat
	switch asyncAPIType {
	case "string":
		return string(literal.STRING), nil
	case "integer":
		return string(literal.INT), nil
	case "number":
		return string(literal.FLOAT), nil
	case "boolean":
		return string(literal.BOOLEAN), nil
	default:
		return "", fmt.Errorf("unknown type: %s", asyncAPIType)
	}
}

func extractFullTypeNameFromRef(ref string) string {
	parsed := strings.Split(ref, "/")
	return parsed[len(parsed)-1]
}

func (c *converter) processProperties(ft *introspection.FullType, schemaRef *openapi3.SchemaRef) error {
	for propertyName, property := range schemaRef.Value.Properties {
		gqlType, err := asyncAPITypeToGQLType(property.Value.Type)
		if err != nil {
			return err
		}
		typeRef, err := getTypeRef(property.Value.Type)
		if err != nil {
			return err
		}
		typeRef.Name = &gqlType
		f := introspection.Field{
			Name: propertyName,
			Type: typeRef,
		}
		ft.Fields = append(ft.Fields, f)
		sort.Slice(ft.Fields, func(i, j int) bool {
			return ft.Fields[i].Name < ft.Fields[j].Name
		})
	}
	return nil
}

func (c *converter) processArray(media *openapi3.MediaType) error {
	fullTypeName := extractFullTypeNameFromRef(media.Schema.Value.Items.Ref)
	_, ok := c.knownFullTypes[fullTypeName]
	if ok {
		return nil
	}
	c.knownFullTypes[fullTypeName] = struct{}{}

	ft := introspection.FullType{
		Kind: introspection.OBJECT,
		Name: fullTypeName,
	}
	for _, item := range media.Schema.Value.Items.Value.AllOf {
		if item.Value.Type == "object" {
			err := c.processProperties(&ft, item)
			if err != nil {
				return err
			}
		}
	}
	c.fullTypes = append(c.fullTypes, ft)
	return nil
}

func (c *converter) processObject(media *openapi3.MediaType) error {
	fullTypeName := extractFullTypeNameFromRef(media.Schema.Ref)
	_, ok := c.knownFullTypes[fullTypeName]
	if ok {
		return nil
	}
	c.knownFullTypes[fullTypeName] = struct{}{}

	ft := introspection.FullType{
		Kind: introspection.OBJECT,
		Name: fullTypeName,
	}
	err := c.processProperties(&ft, media.Schema)
	if err != nil {
		return err
	}
	c.fullTypes = append(c.fullTypes, ft)
	return nil
}

func (c *converter) processContent(media *openapi3.MediaType) error {
	if media.Schema.Value.Type == "array" {
		return c.processArray(media)
	} else if media.Schema.Value.Type == "object" {
		return c.processObject(media)
	}
	return nil
}

func (c *converter) importFullTypes() ([]introspection.FullType, error) {
	for _, pathValue := range c.openapi.Paths {
		for _, response := range pathValue.Get.Responses {
			media, ok := response.Value.Content["application/json"]
			if !ok {
				return nil, errors.New("only application/json is supported")
			}
			err := c.processContent(media)
			if err != nil {
				return nil, err
			}
		}
	}
	return c.fullTypes, nil
}

func (c *converter) importQueryType() (*introspection.FullType, error) {
	// Query root type must be provided. We add an empty Query type with a dummy field.
	queryType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Query",
	}
	return queryType, nil
}

func ImportParsedOpenAPIv3Document(document *openapi3.T, report *operationreport.Report) *ast.Document {
	c := &converter{
		openapi:        document,
		knownFullTypes: make(map[string]struct{}),
		fullTypes:      make([]introspection.FullType, 0),
	}
	data := introspection.Data{}

	data.Schema.QueryType = &introspection.TypeName{
		Name: "Query",
	}
	queryType, err := c.importQueryType()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	data.Schema.Types = append(data.Schema.Types, *queryType)

	fullTypes, err := c.importFullTypes()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	data.Schema.Types = append(data.Schema.Types, fullTypes...)

	outputPretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		report.AddInternalError(err)
		return nil
	}

	jc := introspection.JsonConverter{}
	buf := bytes.NewBuffer(outputPretty)
	doc, err := jc.GraphQLDocument(buf)
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	return doc
}

func ImportOpenAPIDocumentByte(input []byte) (*ast.Document, operationreport.Report) {
	report := operationreport.Report{}
	loader := openapi3.NewLoader()
	parsed, err := loader.LoadFromData(input)
	if err != nil {
		report.AddInternalError(err)
		return nil, report
	}
	return ImportParsedOpenAPIv3Document(parsed, &report), report
}

func ImportOpenAPIDocumentString(input string) (*ast.Document, operationreport.Report) {
	return ImportOpenAPIDocumentByte([]byte(input))
}
