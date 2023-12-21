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

// tryMakeTypeNameFromOperation generates a new type name for unnamed objects based on the status code and operation.
// If the response schema is an object, it returns a type name generated from the current path name. Otherwise, it returns "String".
func (c *converter) tryMakeTypeNameFromOperation(status int, operation *openapi3.Operation) string {
	// Try to make a new type name for unnamed objects.
	responseRef := operation.Responses.Get(status)
	if responseRef != nil && responseRef.Value != nil {
		mediaType := responseRef.Value.Content.Get("application/json")
		if mediaType != nil && mediaType.Schema != nil && mediaType.Schema.Value != nil {
			if mediaType.Schema.Value.Type == "object" {
				return MakeTypeNameFromPathName(c.currentPathName)
			}
		}
	}
	// IBM/openapi-to-graphql uses String as return type.
	// TODO: https://stackoverflow.com/questions/44737043/is-it-possible-to-not-return-any-data-when-using-a-graphql-mutation/44773532#44773532
	return "String"
}

// getInputValueFromRequestBody retrieves the input value from the request body and adds it to the field arguments.
func (c *converter) getInputValueFromRequestBody(field *introspection.Field, status int, operation *openapi3.Operation) error {
	var typeName string
	schema := getJSONSchemaFromRequestBody(operation)
	if schema == nil {
		typeName = c.tryMakeTypeNameFromOperation(status, operation)
	}
	typeName, err := c.getReturnType(schema)
	if err != nil {
		return err
	}
	inputValue, err := c.getInputValue(typeName, schema)
	if err != nil {
		return err
	}
	if operation.RequestBody.Value.Required {
		inputValue.Type = convertToNonNull(&inputValue.Type)
	}
	field.Args = append(field.Args, *inputValue)
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

				var typeName string
				schema := getJSONSchema(status, operation)
				if schema == nil {
					typeName = c.tryMakeTypeNameFromOperation(status, operation)
				} else {
					typeName, err = c.getReturnType(schema)
					if err != nil {
						return nil, err
					}
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

				if operation.RequestBody != nil {
					if err = c.getInputValueFromRequestBody(&f, status, operation); err != nil {
						return nil, err
					}
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
