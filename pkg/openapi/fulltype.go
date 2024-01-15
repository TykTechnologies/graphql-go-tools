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

// checkAndProcessAllOfKeyword checks for the "allOf" keyword in the schema and processes it if it exists.
// It merges the fields, enum values, input fields, possible types, and interfaces of the allOf schemas into one merged type.
// The merged type is then added to the list of full types and stored in the knownFullTypes map.
func (c *converter) checkAndProcessAllOfKeyword(schema *openapi3.SchemaRef) error {
	if schema.Value.AllOf == nil {
		return nil
	}

	var typeName = MakeTypeNameFromPathName(c.currentPathName)
	if _, ok := c.knownFullTypes[typeName]; ok {
		// Already created, passing it.
		return nil
	}

	cc := newConverter(c.openapi)
	for _, allOfSchema := range schema.Value.AllOf {
		if err := cc.processSchema(allOfSchema); err != nil {
			return err
		}
	}
	mergedType := introspection.FullType{
		Kind: introspection.OBJECT,
		Name: typeName,
	}
	knownFields := make(map[string]struct{})
	knownEnumValues := make(map[string]struct{})
	knownInputFields := make(map[string]struct{})
	for _, fullType := range cc.fullTypes {
		for _, field := range fullType.Fields {
			if _, ok := knownFields[field.Name]; !ok {
				knownFields[field.Name] = struct{}{}
				mergedType.Fields = append(mergedType.Fields, field)
			}
		}
		for _, enumValue := range fullType.EnumValues {
			if _, ok := knownEnumValues[enumValue.Name]; !ok {
				knownEnumValues[enumValue.Name] = struct{}{}
				mergedType.EnumValues = append(mergedType.EnumValues, enumValue)
			}
		}
		for _, inputField := range fullType.InputFields {
			if _, ok := knownEnumValues[inputField.Name]; !ok {
				knownInputFields[inputField.Name] = struct{}{}
				mergedType.InputFields = append(mergedType.InputFields, inputField)
			}
		}
		mergedType.PossibleTypes = append(mergedType.PossibleTypes, fullType.PossibleTypes...)
		mergedType.Interfaces = append(mergedType.Interfaces, fullType.Interfaces...)
	}

	sort.Slice(mergedType.Fields, func(i, j int) bool {
		return mergedType.Fields[i].Name < mergedType.Fields[j].Name
	})
	sort.Slice(mergedType.InputFields, func(i, j int) bool {
		return mergedType.InputFields[i].Name < mergedType.InputFields[j].Name
	})
	sort.Slice(mergedType.EnumValues, func(i, j int) bool {
		return mergedType.EnumValues[i].Name < mergedType.EnumValues[j].Name
	})

	c.fullTypes = append(c.fullTypes, mergedType)
	sort.Slice(c.fullTypes, func(i, j int) bool {
		return c.fullTypes[i].Name < c.fullTypes[j].Name
	})
	c.knownFullTypes[mergedType.Name] = &knownFullTypeDetails{}
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

	err = c.checkAndProcessAllOfKeyword(schema)
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
