package introspection_datasource

import (
	"context"
	"encoding/json"
	"io"

	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
)

var (
	null = []byte("null")
)

// Type and RestrictionList struct have been copied from the graphql package to fix
// the import cycle problem.

type Type struct {
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
}

type RestrictionList struct {
	Types []Type
}

type Source struct {
	introspectionData *introspection.Data
	restrictionList   *RestrictionList
}

func (s *Source) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	var req introspectionInput
	if err := json.Unmarshal(input, &req); err != nil {
		return err
	}

	switch req.RequestType {
	case TypeRequestType:
		return s.singleType(w, req.TypeName)
	case TypeEnumValuesRequestType:
		return s.enumValuesForType(w, req.OnTypeName, req.IncludeDeprecated)
	case TypeFieldsRequestType:
		return s.fieldsForType(w, req.OnTypeName, req.IncludeDeprecated)
	}

	/*
			// The following query hits here.
			{
			  __schema {
			    types {
			      name
			    }
			  }
			}

			"Country" type is restricted with its all fields but "Continent" will be
		     shown in the introspection result. Because we just restricted the "name"
		    field of "Continent".
	*/
	var fullTypes []introspection.FullType
	for _, fullType := range s.introspectionData.Schema.Types {
		if !s.isTypeRestricted(fullType.Name) {
			fullTypes = append(fullTypes, fullType)
		}
	}
	s.introspectionData.Schema.Types = fullTypes
	return json.NewEncoder(w).Encode(s.introspectionData.Schema)
}

func (s *Source) isTypeRestricted(name string) bool {
	if s.restrictionList == nil {
		return false
	}
	for _, t := range s.restrictionList.Types {
		if len(t.Fields) != 0 {
			continue
		}
		if t.Name == name {
			return true
		}
	}
	return false
}

func (s *Source) isFieldRestricted(typeName, fieldName string) bool {
	if s.restrictionList == nil {
		return false
	}
	for _, t := range s.restrictionList.Types {
		if t.Name == typeName {
			for _, field := range t.Fields {
				if field == fieldName {
					return true
				}
			}
		}
	}
	return false
}

func (s *Source) typeInfo(typeName *string) *introspection.FullType {
	if typeName == nil {
		return nil
	}

	for _, fullType := range s.introspectionData.Schema.Types {
		if s.isTypeRestricted(fullType.Name) {
			continue
		}
		if fullType.Name == *typeName {
			return &fullType
		}
	}
	return nil
}

func (s *Source) writeNull(w io.Writer) error {
	_, err := w.Write(null)
	return err
}

func (s *Source) singleType(w io.Writer, typeName *string) error {
	typeInfo := s.typeInfo(typeName)
	if typeInfo == nil {
		return s.writeNull(w)
	}

	return json.NewEncoder(w).Encode(typeInfo)
}

func (s *Source) fieldsForType(w io.Writer, typeName *string, includeDeprecated bool) error {
	typeInfo := s.typeInfo(typeName)
	if typeInfo == nil {
		return s.writeNull(w)
	}

	if includeDeprecated {
		return json.NewEncoder(w).Encode(typeInfo.Fields)
	}

	fields := make([]introspection.Field, 0, len(typeInfo.Fields))
	for _, field := range typeInfo.Fields {
		if s.isFieldRestricted(*typeName, field.Name) {
			continue
		}
		if !field.IsDeprecated {
			fields = append(fields, field)
		}
	}

	return json.NewEncoder(w).Encode(fields)
}

func (s *Source) enumValuesForType(w io.Writer, typeName *string, includeDeprecated bool) error {
	typeInfo := s.typeInfo(typeName)
	if typeInfo == nil {
		return s.writeNull(w)
	}

	if includeDeprecated {
		return json.NewEncoder(w).Encode(typeInfo.EnumValues)
	}

	enumValues := make([]introspection.EnumValue, 0, len(typeInfo.EnumValues))
	for _, enumValue := range typeInfo.EnumValues {
		if !enumValue.IsDeprecated {
			enumValues = append(enumValues, enumValue)
		}
	}

	return json.NewEncoder(w).Encode(enumValues)
}
