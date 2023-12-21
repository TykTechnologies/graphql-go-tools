package openapi

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

// getInputValueFromParameter retrieves the input value from the given parameter and adds it to the field arguments.
// If the parameter is required, the input value is converted to a non-null type.
func (c *converter) getInputValueFromParameter(field *introspection.Field, parameter *openapi3.ParameterRef) error {
	inputValue, err := c.getInputValue(parameter.Value.Name, parameter.Value.Schema)
	if err != nil {
		return err
	}
	if parameter.Value.Required {
		inputValue.Type = convertToNonNull(&inputValue.Type)
	}
	field.Args = append(field.Args, *inputValue)
	return nil
}

func (c *converter) getInputValuesFromParameters(field *introspection.Field, parameters openapi3.Parameters) error {
	for _, parameter := range parameters {
		if err := c.getInputValueFromParameter(field, parameter); err != nil {
			return err
		}
	}
	return nil
}

func (c *converter) importMutationType() (*introspection.FullType, error) {
	mutationType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Mutation",
	}

	for pathName, pathItem := range c.openapi.Paths {
		c.currentPathName = pathName
		c.currentPathItem = pathItem
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			operation := pathItem.GetOperation(method)
			if operation == nil {
				continue
			}
			for statusCodeStr := range operation.Responses {
				if statusCodeStr == "default" {
					continue
				}
				status, err := strconv.Atoi(statusCodeStr)
				if err != nil {
					return nil, err
				}

				if !isValidResponse(status) {
					continue
				}

				// TODO: try to remove this block
				/*typeName, err := extractTypeName(status, operation)
				if errors.Is(err, errTypeNameExtractionImpossible) {
					// Try to make a new type name for unnamed objects.
					responseRef := operation.Responses.Get(status)
					if responseRef != nil && responseRef.Value != nil {
						mediaType := responseRef.Value.Content.Get("application/json")
						if mediaType != nil && mediaType.Schema != nil && mediaType.Schema.Value != nil {
							if mediaType.Schema.Value.Type == "object" {
								typeName = MakeTypeNameFromPathName(c.currentPathName)
								err = nil
							}
						}
					}

					if typeName == "" {
						// IBM/openapi-to-graphql uses String as return type.
						// TODO: https://stackoverflow.com/questions/44737043/is-it-possible-to-not-return-any-data-when-using-a-graphql-mutation/44773532#44773532
						typeName = "String"
						err = nil
					}
				}
				if err != nil {
					return nil, err
				}*/

				schema := getJSONSchema(status, operation)
				if schema == nil {
					continue
				}
				kind := schema.Value.Type
				if kind == "" {
					// We assume that it is an object type.
					kind = "object"
				}
				typeName, err := c.getReturnType(schema)
				if err != nil {
					return nil, err
				}

				typeName = strcase.ToCamel(typeName)
				typeRef, err := getTypeRef("object")
				if err != nil {
					return nil, err
				}
				typeRef.Name = &typeName

				f := introspection.Field{
					Name:        MakeFieldNameFromOperationID(operation.OperationID),
					Type:        typeRef,
					Description: getOperationDescription(operation),
				}
				if f.Name == "" {
					f.Name = MakeFieldNameFromEndpoint(method, pathName)
				}

				var inputValue *introspection.InputValue
				if operation.RequestBody != nil {
					schema := getJSONSchemaFromRequestBody(operation)
					fullTypeName, err := extractFullTypeNameFromRef(schema.Ref)
					if err != nil {
						return nil, err
					}
					inputValue, err = c.getInputValue(fullTypeName, schema)
					if err != nil {
						return nil, err
					}
					if operation.RequestBody.Value.Required {
						inputValue.Type = convertToNonNull(&inputValue.Type)
					}
					f.Args = append(f.Args, *inputValue)
				}

				if err = c.getInputValuesFromParameters(&f, operation.Parameters); err != nil {
					return nil, err
				}
				if err = c.getInputValuesFromParameters(&f, c.currentPathItem.Parameters); err != nil {
					return nil, err
				}

				sort.Slice(f.Args, func(i, j int) bool {
					return f.Args[i].Name < f.Args[j].Name
				})
				mutationType.Fields = append(mutationType.Fields, f)
			}
		}
	}
	sort.Slice(mutationType.Fields, func(i, j int) bool {
		return mutationType.Fields[i].Name < mutationType.Fields[j].Name
	})
	return mutationType, nil
}
