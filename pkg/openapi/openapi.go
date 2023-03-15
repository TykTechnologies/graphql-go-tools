package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
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

func getGraphQLType(openapiType string) (string, error) {
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

func (c *converter) processSchemaProperties(fullType *introspection.FullType, schemaRef *openapi3.SchemaRef) error {
	for propertyName, property := range schemaRef.Value.Properties {
		gqlType, err := getGraphQLType(property.Value.Type)
		if err != nil {
			return err
		}
		typeRef, err := getTypeRef(property.Value.Type)
		if err != nil {
			return err
		}
		typeRef.Name = &gqlType
		field := introspection.Field{
			Name: propertyName,
			Type: typeRef,
		}

		fullType.Fields = append(fullType.Fields, field)
		sort.Slice(fullType.Fields, func(i, j int) bool {
			return fullType.Fields[i].Name < fullType.Fields[j].Name
		})
	}
	return nil
}

func (c *converter) processInputFields(ft *introspection.FullType, schemaRef *openapi3.SchemaRef) error {
	for propertyName, property := range schemaRef.Value.Properties {
		gqlType, err := getGraphQLType(property.Value.Type)
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

func (c *converter) processArray(schema *openapi3.SchemaRef) error {
	fullTypeName := extractFullTypeNameFromRef(schema.Value.Items.Ref)
	_, ok := c.knownFullTypes[fullTypeName]
	if ok {
		return nil
	}
	c.knownFullTypes[fullTypeName] = struct{}{}

	ft := introspection.FullType{
		Kind: introspection.OBJECT,
		Name: fullTypeName,
	}
	for _, item := range schema.Value.Items.Value.AllOf {
		if item.Value.Type == "object" {
			err := c.processSchemaProperties(&ft, item)
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
	err := c.processSchemaProperties(&ft, schema)
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

func (c *converter) processSchema(schema *openapi3.SchemaRef) error {
	if schema.Value.Type == "array" {
		return c.processArray(schema)
	} else if schema.Value.Type == "object" {
		return c.processObject(schema)
	}
	return nil
}

func (c *converter) importFullTypes() ([]introspection.FullType, error) {
	for _, openapiPaths := range c.openapi.Paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPut} {
			operation := openapiPaths.GetOperation(method)
			if operation == nil {
				continue
			}

			defaultJSONSchema := getDefaultJSONSchema(operation)
			err := c.processSchema(defaultJSONSchema)
			if err != nil {
				return nil, err
			}

			for statusCodeStr := range operation.Responses {
				if statusCodeStr == "default" {
					continue
				}
				status, err := strconv.Atoi(statusCodeStr)
				if err != nil {
					return nil, err
				}
				schema := getJSONSchema(status, operation)
				if schema == nil {
					continue
				}
				err = c.processSchema(schema)
				if err != nil {
					return nil, err
				}
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

func getJSONSchemaFromResponseRef(response *openapi3.ResponseRef) *openapi3.SchemaRef {
	var schema *openapi3.SchemaRef
	for _, contentType := range []string{"application/json", "application/geo+json"} {
		content := response.Value.Content.Get(contentType)
		if content != nil {
			return content.Schema
		}
	}
	return schema
}

func getDefaultJSONSchema(operation *openapi3.Operation) *openapi3.SchemaRef {
	return getJSONSchemaFromResponseRef(operation.Responses.Default())
}

func getJSONSchema(status int, operation *openapi3.Operation) *openapi3.SchemaRef {
	response := operation.Responses.Get(status)
	if response == nil {
		return nil
	}
	return getJSONSchemaFromResponseRef(response)
}

func (c *converter) importQueryTypeFields(typeRef *introspection.TypeRef, operation *openapi3.Operation) (*introspection.Field, error) {
	f := introspection.Field{
		Name: strcase.ToLowerCamel(operation.OperationID),
		Type: *typeRef,
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

		gqlType, err := getGraphQLType(paramType)
		if err != nil {
			return nil, err
		}

		if parameter.Value.Schema.Value.Items != nil {
			ofType := parameter.Value.Schema.Value.Items.Value.Type
			ofTypeRef, err := getTypeRef(ofType)
			if err != nil {
				return nil, err
			}
			typeRef.OfType = &ofTypeRef
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
	return &f, nil
}

func (c *converter) importQueryType() (*introspection.FullType, error) {
	queryType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Query",
	}
	for _, openapiPath := range c.openapi.Paths {
		for _, method := range []string{http.MethodGet} {
			operation := openapiPath.GetOperation(method)
			if operation == nil {
				// We only support HTTP GET operation.
				continue
			}

			kind := getJSONSchema(http.StatusOK, operation).Value.Type
			if kind == "" {
				// TODO: why?
				kind = "object"
			}

			typeName := strcase.ToCamel(extractTypeName(http.StatusOK, operation))
			typeRef, err := getTypeRef(kind)
			if err != nil {
				return nil, err
			}
			if kind == "array" {
				// Array of some type
				typeRef.OfType = &introspection.TypeRef{Kind: 3, Name: &typeName}
			}

			typeRef.Name = &typeName
			queryField, err := c.importQueryTypeFields(&typeRef, operation)
			if err != nil {
				return nil, err
			}
			queryType.Fields = append(queryType.Fields, *queryField)
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
		gqlType, err = getGraphQLType(paramType)
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

	data.Schema.MutationType = &introspection.TypeName{
		Name: "Mutation",
	}
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
