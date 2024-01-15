package openapi

import (
	"fmt"
	"sort"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
)

func (c *converter) processInputFields(ft *introspection.FullType, schemaRef *openapi3.SchemaRef) error {
	for name, property := range schemaRef.Value.Properties {
		typeRef, err := c.makeTypeRefFromSchemaRef(property, name, true, isNonNullable(name, schemaRef.Value.Required))
		if err != nil {
			return err
		}
		f := introspection.InputValue{
			Name: name,
			Type: *typeRef,
		}
		ft.InputFields = append(ft.InputFields, f)
		sort.Slice(ft.InputFields, func(i, j int) bool {
			return ft.InputFields[i].Name < ft.InputFields[j].Name
		})
	}
	return nil
}

func (c *converter) processInputObject(schema *openapi3.SchemaRef) error {
	fullTypeName := MakeInputTypeName(schema.Ref)
	_, ok := c.knownFullTypes[fullTypeName]
	if ok {
		return nil
	}
	c.knownFullTypes[fullTypeName] = &knownFullTypeDetails{}

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

// makeInputObjectFromAllOf converts a schema with multiple "allOf" properties into an input object.
func (c *converter) makeInputObjectFromAllOf(schema *openapi3.SchemaRef) (string, error) {
	cc := newConverter(c.openapi)
	for _, allOfSchema := range schema.Value.AllOf {
		if err := cc.processSchema(allOfSchema); err != nil {
			return "", err
		}
	}
	mergedType := introspection.FullType{
		Kind: introspection.INPUTOBJECT,
		Name: MakeInputTypeName(MakeTypeNameFromPathName(c.currentPathName)),
	}
	knownFields := make(map[string]struct{})
	knownEnumValues := make(map[string]struct{})
	knownInputFields := make(map[string]struct{})
	for _, fullType := range cc.fullTypes {
		for _, field := range fullType.Fields {
			if _, ok := knownFields[field.Name]; !ok {
				knownFields[field.Name] = struct{}{}
				// Convert a Field to a InputValue
				inputValue := introspection.InputValue{
					Name:        field.Name,
					Description: field.Description,
					Type:        field.Type,
				}
				mergedType.InputFields = append(mergedType.InputFields, inputValue)
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

	return mergedType.Name, nil
}

func (c *converter) getInputValue(name string, schema *openapi3.SchemaRef) (*introspection.InputValue, error) {
	var (
		err     error
		gqlType string
		typeRef introspection.TypeRef
	)

	if len(schema.Value.Enum) > 0 {
		enumType := c.createOrGetEnumType(name, schema)
		typeRef = getEnumTypeRef()
		gqlType = enumType.Name
	} else if schema.Value.OneOf != nil && len(schema.Value.OneOf) > 0 {
		gqlType = name // JSON
		typeRef = introspection.TypeRef{Kind: 0}
	} else if schema.Value.AllOf != nil && len(schema.Value.AllOf) > 0 {
		gqlType, err = c.makeInputObjectFromAllOf(schema)
		if err != nil {
			return nil, err
		}
		typeRef = introspection.TypeRef{Kind: 7}
	} else {
		paramType := schema.Value.Type
		if paramType == "array" {
			paramType = schema.Value.Items.Value.Type
		}

		typeRef, err = getParamTypeRef(paramType)
		if err != nil {
			return nil, err
		}

		gqlType = name
		if paramType != "object" {
			gqlType, err = getPrimitiveGraphQLTypeName(paramType)
			if err != nil {
				return nil, err
			}
		} else {
			name = MakeInputTypeName(name)
			gqlType = name
			err = c.processInputObject(schema)
			if err != nil {
				return nil, err
			}
		}
	}

	if schema.Value.Items != nil {
		ofType := schema.Value.Items.Value.Type
		ofTypeRef, err := getParamTypeRef(ofType)
		if err != nil {
			return nil, err
		}
		typeRef.OfType = &ofTypeRef
		gqlType = fmt.Sprintf("[%s]", gqlType)
	}

	typeRef.Name = &gqlType
	return &introspection.InputValue{
		Name: MakeParameterName(name),
		Type: typeRef,
	}, nil
}
