package openapi

import (
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

func getEnumTypeRef() introspection.TypeRef {
	return introspection.TypeRef{Kind: 4}
}

func (c *converter) createOrGetEnumType(name string, schema *openapi3.SchemaRef) *introspection.FullType {
	name = strcase.ToCamel(name)
	if enumType, ok := c.knownEnums[name]; ok {
		return enumType
	}

	enumType := &introspection.FullType{
		Kind: introspection.ENUM,
		Name: name,
	}

	for _, enum := range schema.Value.Enum {
		enumValue := introspection.EnumValue{
			Name: strings.ToUpper(strcase.ToSnake(enum.(string))),
		}
		enumType.EnumValues = append(enumType.EnumValues, enumValue)
	}
	c.fullTypes = append(c.fullTypes, *enumType)
	c.knownEnums[name] = enumType
	return enumType
}
