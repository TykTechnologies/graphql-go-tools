package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/pkg/lexer/literal"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
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

func getParamTypeRef(kind string) (introspection.TypeRef, error) {
	// See introspection_enum.go
	switch kind {
	case "string", "integer", "number", "boolean":
		return introspection.TypeRef{Kind: 0}, nil
	case "object":
		// InputType
		return introspection.TypeRef{Kind: 7}, nil
	case "array":
		return introspection.TypeRef{Kind: 1}, nil
	}
	return introspection.TypeRef{}, errors.New("unknown type")
}

func openAPITypeToGQLType(openapiType string) (string, error) {
	switch openapiType {
	case "string":
		return string(literal.STRING), nil
	case "integer":
		return string(literal.INT), nil
	case "number":
		return string(literal.FLOAT), nil
	case "boolean":
		return string(literal.BOOLEAN), nil
	default:
		return "", fmt.Errorf("unknown type: %s", openapiType)
	}
}

func extractFullTypeNameFromRef(ref string) string {
	parsed := strings.Split(ref, "/")
	return parsed[len(parsed)-1]
}

func (c *converter) processProperties(ft *introspection.FullType, schemaRef *openapi3.SchemaRef) error {
	for propertyName, property := range schemaRef.Value.Properties {
		gqlType, err := openAPITypeToGQLType(property.Value.Type)
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

func (c *converter) processInputFields(ft *introspection.FullType, schemaRef *openapi3.SchemaRef) error {
	for propertyName, property := range schemaRef.Value.Properties {
		gqlType, err := openAPITypeToGQLType(property.Value.Type)
		if err != nil {
			return err
		}
		typeRef, err := getTypeRef(property.Value.Type)
		if err != nil {
			return err
		}

		typeRef.Name = &gqlType
		f := introspection.InputValue{
			Name: propertyName,
			Type: typeRef,
		}
		ft.InputFields = append(ft.InputFields, f)
		sort.Slice(ft.InputFields, func(i, j int) bool {
			return ft.InputFields[i].Name < ft.InputFields[j].Name
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

func (c *converter) processObject(schema *openapi3.SchemaRef) error {
	fullTypeName := extractFullTypeNameFromRef(schema.Ref)
	_, ok := c.knownFullTypes[fullTypeName]
	if ok {
		return nil
	}
	c.knownFullTypes[fullTypeName] = struct{}{}

	ft := introspection.FullType{
		Kind: introspection.OBJECT,
		Name: fullTypeName,
	}
	err := c.processProperties(&ft, schema)
	if err != nil {
		return err
	}
	c.fullTypes = append(c.fullTypes, ft)
	return nil
}

func (c *converter) processInputObject(schema *openapi3.SchemaRef) error {
	fullTypeName := extractFullTypeNameFromRef(schema.Ref)
	_, ok := c.knownFullTypes[fullTypeName]
	if ok {
		return nil
	}
	c.knownFullTypes[fullTypeName] = struct{}{}

	ft := introspection.FullType{
		Kind: introspection.INPUTOBJECT,
		Name: fullTypeName,
	}
	err := c.processInputFields(&ft, schema)
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
		return c.processObject(media.Schema)
	}
	return nil
}

func (c *converter) importFullTypes() ([]introspection.FullType, error) {
	for _, openapiPaths := range c.openapi.Paths {
		for _, response := range openapiPaths.Get.Responses {
			mediaType := response.Value.Content.Get("application/json")
			if mediaType == nil {
				return nil, errors.New("only application/json is supported")
			}
			err := c.processContent(mediaType)
			if err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(c.fullTypes, func(i, j int) bool {
		return c.fullTypes[i].Name < c.fullTypes[j].Name
	})
	return c.fullTypes, nil
}

func extractTypeName(status int, operation *openapi3.Operation) string {
	response := operation.Responses.Get(status)
	if response == nil {
		// Nil response?
		return ""
	}
	schema := response.Value.Content.Get("application/json")
	if schema == nil && len(response.Value.Content) == 0 {
		return ""
	}
	if schema.Schema.Value.Type == "array" {
		return extractFullTypeNameFromRef(schema.Schema.Value.Items.Ref)
	}
	return extractFullTypeNameFromRef(schema.Schema.Ref)
}

func getJSONSchema(status int, operation *openapi3.Operation) *openapi3.SchemaRef {
	var schema *openapi3.SchemaRef
	for _, contentType := range []string{"application/json", "application/geo+json"} {
		content := operation.Responses.Get(status).Value.Content.Get(contentType)
		if content != nil {
			return content.Schema
		}
	}
	return schema
}

func (c *converter) importQueryType() (*introspection.FullType, error) {
	// Query root type must be provided. We add an empty Query type with a dummy field.
	queryType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Query",
	}
	for _, openapiPath := range c.openapi.Paths {
		for _, method := range []string{"GET"} {
			operation := openapiPath.GetOperation(method)
			if operation == nil {
				continue
			}
			kind := getJSONSchema(200, operation).Value.Type
			if kind == "" {
				kind = "object"
			}
			typeName := strcase.ToCamel(extractTypeName(200, operation))

			typeRef, err := getTypeRef(kind)
			if err != nil {
				return nil, err
			}
			if kind == "array" {
				typeRef.OfType = &introspection.TypeRef{Kind: 3, Name: &typeName}
			}

			typeRef.Name = &typeName
			f := introspection.Field{
				Name: strcase.ToLowerCamel(operation.OperationID),
				Type: typeRef,
			}

			for _, parameter := range operation.Parameters {
				paramType := parameter.Value.Schema.Value.Type
				if paramType == "array" {
					paramType = parameter.Value.Schema.Value.Items.Value.Type
				}

				typeRef, err := getTypeRef(paramType)
				if err != nil {
					return nil, err
				}

				gqlType, err := openAPITypeToGQLType(paramType)
				if err != nil {
					return nil, err
				}

				if parameter.Value.Schema.Value.Items != nil {
					otype := parameter.Value.Schema.Value.Items.Value.Type
					ot, err := getTypeRef(otype)
					if err != nil {
						return nil, err
					}
					typeRef.OfType = &ot
					gqlType = fmt.Sprintf("[%s]", gqlType)
				}

				typeRef.Name = &gqlType
				iv := introspection.InputValue{
					Name: parameter.Value.Name,
					Type: typeRef,
				}
				f.Args = append(f.Args, iv)
				sort.Slice(f.Args, func(i, j int) bool {
					return f.Args[i].Name < f.Args[j].Name
				})
			}
			queryType.Fields = append(queryType.Fields, f)
		}
	}
	sort.Slice(queryType.Fields, func(i, j int) bool {
		return queryType.Fields[i].Name < queryType.Fields[j].Name
	})
	return queryType, nil
}

func (c *converter) addParameters(name string, schema *openapi3.SchemaRef) (*introspection.InputValue, error) {
	paramType := schema.Value.Type
	if paramType == "array" {
		paramType = schema.Value.Items.Value.Type
	}

	typeRef, err := getParamTypeRef(paramType)
	if err != nil {
		return nil, err
	}

	gqlType := name
	if paramType != "object" {
		gqlType, err = openAPITypeToGQLType(paramType)
		if err != nil {
			return nil, err
		}
	} else {
		err = c.processInputObject(schema)
		if err != nil {
			return nil, err
		}
	}

	if schema.Value.Items != nil {
		otype := schema.Value.Items.Value.Type
		ot, err := getParamTypeRef(otype)
		if err != nil {
			return nil, err
		}
		typeRef.OfType = &ot
		gqlType = fmt.Sprintf("[%s]", gqlType)
	}

	typeRef.Name = &gqlType
	return &introspection.InputValue{
		Name: strcase.ToLowerCamel(name),
		Type: typeRef,
	}, nil
}

func (c *converter) importMutationType() (*introspection.FullType, error) {
	// Query root type must be provided. We add an empty Query type with a dummy field.
	mutationType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Mutation",
	}
	for _, openapiPath := range c.openapi.Paths {
		for _, method := range []string{"POST", "PUT", "DELETE"} {
			operation := openapiPath.GetOperation(method)
			if operation == nil {
				continue
			}
			// TODO: We can pick only one response type in UDG.
			status := 200
			if method == "DELETE" {
				status = 204
			}
			typeName := strcase.ToCamel(extractTypeName(status, operation))
			if typeName == "" {
				// TODO: https://stackoverflow.com/questions/44737043/is-it-possible-to-not-return-any-data-when-using-a-graphql-mutation/44773532#44773532
				typeName = "Boolean"
			}

			typeRef, err := getTypeRef("object")
			if err != nil {
				return nil, err
			}
			typeRef.Name = &typeName
			f := introspection.Field{
				Name: strcase.ToLowerCamel(operation.OperationID),
				Type: typeRef,
			}

			var inputValue *introspection.InputValue
			if operation.RequestBody != nil {
				// TODO: Find a new way to get schema
				content := operation.RequestBody.Value.Content.Get("application/json")
				if content == nil {
					content = operation.RequestBody.Value.Content.Get("application/x-www-form-urlencoded")
				}
				schema := content.Schema
				inputValue, err = c.addParameters(extractFullTypeNameFromRef(schema.Ref), schema)
				if err != nil {
					return nil, err
				}
				f.Args = append(f.Args, *inputValue)
			} else {
				for _, parameter := range operation.Parameters {
					inputValue, err = c.addParameters(parameter.Value.Name, parameter.Value.Schema)
					if err != nil {
						return nil, err
					}
					f.Args = append(f.Args, *inputValue)
				}
			}
			sort.Slice(f.Args, func(i, j int) bool {
				return f.Args[i].Name < f.Args[j].Name
			})
			mutationType.Fields = append(mutationType.Fields, f)
		}
	}
	return mutationType, nil
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

	mutationType, err := c.importMutationType()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	data.Schema.Types = append(data.Schema.Types, *mutationType)

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
	loader.IsExternalRefsAllowed = true
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
