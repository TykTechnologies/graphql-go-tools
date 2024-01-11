package openapi

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
)

func (c *converter) checkAndProcessOneOfKeyword(schema *openapi3.SchemaRef) error {
	if schema.Value.OneOf == nil {
		return nil
	}
	for _, oneOfSchema := range schema.Value.OneOf {
		err := c.processSchema(oneOfSchema)
		if err != nil {
			return err
		}
	}
	// Create a UNION type here
	if len(schema.Value.OneOf) > 0 {
		unionName := MakeTypeNameFromPathName(c.currentPathName)
		if _, ok := c.knownUnions[unionName]; ok {
			// Already have the union definition.
			// TODO: Do we need to add more types to this UNION?
			return nil
		}
		unionType := &introspection.FullType{
			Kind:          introspection.UNION,
			Name:          unionName,
			PossibleTypes: []introspection.TypeRef{},
		}
		for _, oneOfSchema := range schema.Value.OneOf {
			fullTypeName, err := extractFullTypeNameFromRef(oneOfSchema.Ref)
			if errors.Is(err, errTypeNameExtractionImpossible) {
				fullTypeName = MakeTypeNameFromPathName(c.currentPathName)
				err = nil
			}
			if err != nil {
				return err
			}
			unionType.PossibleTypes = append(unionType.PossibleTypes, introspection.TypeRef{
				Kind: introspection.OBJECT,
				Name: &fullTypeName,
			})
		}
		c.fullTypes = append(c.fullTypes, *unionType)
	}
	return nil
}

func (c *converter) processSchema(schema *openapi3.SchemaRef) error {
	if schema.Value.Type == "array" {
		arrayOf := schema.Value.Items.Value.Type
		if arrayOf == "string" || arrayOf == "integer" || arrayOf == "number" || arrayOf == "boolean" {
			return nil
		}
		return c.processArray(schema)
	} else if schema.Value.Type == "object" {
		return c.processObject(schema)
	}

	err := c.checkAndProcessOneOfKeyword(schema)
	if err != nil {
		return err
	}

	return nil
}

func (c *converter) importFullTypes() ([]introspection.FullType, error) {
	for pathName, pathItem := range c.openapi.Paths {
		c.currentPathName = pathName
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPut} {
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

func (c *converter) updateFullTypeDetails(schema *openapi3.SchemaRef, typeName string) (ok bool) {
	var introspectionFullType *introspection.FullType
	for i := 0; i < len(c.fullTypes); i++ {
		if c.fullTypes[i].Name == typeName {
			introspectionFullType = &c.fullTypes[i]
			break
		}
	}

	if introspectionFullType == nil {
		return false
	}

	if !c.knownFullTypes[typeName].hasDescription {
		introspectionFullType.Description = schema.Value.Description
		c.knownFullTypes[typeName].hasDescription = true
	}

	return true
}

// checkForNewKnownFullTypeDetails will return `true` if the `openapi3.SchemaRef` contains new type details and `false` if not.
func checkForNewKnownFullTypeDetails(schema *openapi3.SchemaRef, currentDetails *knownFullTypeDetails) bool {
	if !currentDetails.hasDescription && len(schema.Value.Description) > 0 {
		return true
	}
	return false
}
