package graphql

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astnormalization"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astparser"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/TykTechnologies/graphql-go-tools/pkg/asttransform"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astvalidation"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/TykTechnologies/graphql-go-tools/pkg/pool"
)

type TypeFields struct {
	TypeName   string
	FieldNames []string
}

type TypeFieldArguments struct {
	TypeName      string
	FieldName     string
	ArgumentNames []string
}

type Schema struct {
	rawInput     []byte
	rawSchema    []byte
	document     ast.Document
	isNormalized bool
	hash         uint64
}

func (s *Schema) Hash() (uint64, error) {
	if s.hash != 0 {
		return s.hash, nil
	}
	h := pool.Hash64.Get()
	h.Reset()
	defer pool.Hash64.Put(h)
	printer := astprinter.Printer{}
	err := printer.Print(&s.document, nil, h)
	if err != nil {
		return 0, err
	}
	s.hash = h.Sum64()
	return s.hash, nil
}

func NewSchemaFromReader(reader io.Reader) (*Schema, error) {
	schemaContent, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return createSchema(schemaContent, true)
}

func NewSchemaFromString(schema string) (*Schema, error) {
	schemaContent := []byte(schema)

	return createSchema(schemaContent, true)
}

func ValidateSchemaString(schema string) (result ValidationResult, err error) {
	parsedSchema, err := NewSchemaFromString(schema)
	if err != nil {
		return ValidationResult{
			Valid: false,
			Errors: SchemaValidationErrors{
				SchemaValidationError{Message: err.Error()},
			},
		}, nil
	}

	return parsedSchema.Validate()
}

func (s *Schema) Normalize() (result NormalizationResult, err error) {
	if s.isNormalized {
		return NormalizationResult{
			Successful: true,
			Errors:     nil,
		}, nil
	}

	report := operationreport.Report{}
	astnormalization.NormalizeDefinition(&s.document, &report)
	if report.HasErrors() {
		return normalizationResultFromReport(report)
	}

	normalizedSchemaBuffer := &bytes.Buffer{}
	err = astprinter.PrintIndent(&s.document, nil, []byte("  "), normalizedSchemaBuffer)
	if err != nil {
		return NormalizationResult{
			Successful: false,
			Errors:     nil,
		}, err
	}

	normalizedSchema, err := createSchema(normalizedSchemaBuffer.Bytes(), false)
	if err != nil {
		return NormalizationResult{
			Successful: false,
			Errors:     nil,
		}, err
	}

	s.rawSchema = normalizedSchema.rawSchema
	s.document = normalizedSchema.document
	s.isNormalized = true
	return NormalizationResult{Successful: true, Errors: nil}, nil
}

func (s *Schema) Input() []byte {
	return s.rawInput
}

func (s *Schema) Document() []byte {
	return s.rawSchema
}

// HasQueryType TODO: should be deprecated?
func (s *Schema) HasQueryType() bool {
	return len(s.document.Index.QueryTypeName) > 0
}

func (s *Schema) QueryTypeName() string {
	return string(s.document.Index.QueryTypeName)
}

func (s *Schema) IsNormalized() bool {
	return s.isNormalized
}

func (s *Schema) HasMutationType() bool {
	return len(s.document.Index.MutationTypeName) > 0
}

func (s *Schema) MutationTypeName() string {
	if !s.HasMutationType() {
		return ""
	}

	return string(s.document.Index.MutationTypeName)
}

func (s *Schema) HasSubscriptionType() bool {
	return len(s.document.Index.SubscriptionTypeName) > 0
}

func (s *Schema) SubscriptionTypeName() string {
	if !s.HasSubscriptionType() {
		return ""
	}

	return string(s.document.Index.SubscriptionTypeName)
}

func (s *Schema) Validate() (result ValidationResult, err error) {
	var report operationreport.Report
	var isValid bool

	validator := astvalidation.DefaultDefinitionValidator()
	validationState := validator.Validate(&s.document, &report)
	if validationState == astvalidation.Valid {
		isValid = true
	}

	return ValidationResult{
		Valid:  isValid,
		Errors: schemaValidationErrorsFromOperationReport(report),
	}, nil
}

// IntrospectionResponse - writes full schema introspection response into writer
func (s *Schema) IntrospectionResponse(out io.Writer) error {
	var (
		introspectionData = struct {
			Data introspection.Data `json:"data"`
		}{}
		report operationreport.Report
	)
	gen := introspection.NewGenerator()
	gen.Generate(&s.document, &report, &introspectionData.Data)
	if report.HasErrors() {
		return report
	}
	return json.NewEncoder(out).Encode(introspectionData)
}

func (s *Schema) GetAllFieldArguments(skipFieldFuncs ...SkipFieldFunc) []TypeFieldArguments {
	objectTypeExtensions := make(map[string]ast.ObjectTypeExtension)
	for _, objectTypeExtension := range s.document.ObjectTypeExtensions {
		typeName, ok := s.typeNameOfObjectTypeIfHavingFields(objectTypeExtension.ObjectTypeDefinition)
		if !ok {
			continue
		}

		objectTypeExtensions[typeName] = objectTypeExtension
	}

	typeFieldArguments := make([]TypeFieldArguments, 0)
	for objRef, objectType := range s.document.ObjectTypeDefinitions {
		typeName, ok := s.typeNameOfObjectTypeIfHavingFields(objectType)
		if !ok {
			continue
		}

		for _, fieldRef := range objectType.FieldsDefinition.Refs {
			fieldName, skip := s.determineIfFieldWithFieldNameShouldBeSkipped(fieldRef, typeName, skipFieldFuncs...)
			if skip {
				continue
			}
			s.addTypeFieldArgsForFieldRef(fieldRef, typeName, fieldName, &typeFieldArguments)
			s.addParentInterfaceFields(objRef, fieldName, &typeFieldArguments, skipFieldFuncs...)
		}

		objectTypeExt, ok := objectTypeExtensions[typeName]
		if !ok {
			continue
		}

		for _, fieldRef := range objectTypeExt.FieldsDefinition.Refs {
			fieldName, skip := s.determineIfFieldWithFieldNameShouldBeSkipped(fieldRef, typeName, skipFieldFuncs...)
			if skip {
				continue
			}

			s.addTypeFieldArgsForFieldRef(fieldRef, typeName, fieldName, &typeFieldArguments)
		}
	}

	return typeFieldArguments
}

func (s *Schema) typeNameOfObjectTypeIfHavingFields(objectType ast.ObjectTypeDefinition) (typeName string, ok bool) {
	if !objectType.HasFieldDefinitions {
		return "", false
	}

	return s.document.Input.ByteSliceString(objectType.Name), true
}

func (s *Schema) fieldNameOfFieldDefinitionIfHavingArguments(field ast.FieldDefinition, ref int) (fieldName string, ok bool) {
	if !field.HasArgumentsDefinitions {
		return "", false
	}

	return s.document.FieldDefinitionNameString(ref), true
}

func (s *Schema) determineIfFieldWithFieldNameShouldBeSkipped(ref int, typeName string, skipFieldFuncs ...SkipFieldFunc) (fieldName string, skip bool) {
	field := s.document.FieldDefinitions[ref]
	fieldName, ok := s.fieldNameOfFieldDefinitionIfHavingArguments(field, ref)
	if !ok {
		return fieldName, true
	}

	for _, skipFieldFunc := range skipFieldFuncs {
		if skipFieldFunc != nil && skipFieldFunc(typeName, fieldName, s.document) {
			skip = true
			break
		}
	}

	return fieldName, skip
}

func (s *Schema) addParentInterfaceFields(objectRef int, fieldName string, fieldArguments *[]TypeFieldArguments, skippedFields ...SkipFieldFunc) {
	objectType := s.document.ObjectTypeDefinitions[objectRef]
	if len(objectType.ImplementsInterfaces.Refs) < 1 {
		return
	}
	// iterate through all interfaces available
	for _, interfaceRef := range objectType.ImplementsInterfaces.Refs {
		// check the interface whose field matches the fieldName, if any
		interfaceName := s.document.ResolveTypeNameString(interfaceRef)
		node, exist := s.document.Index.FirstNodeByNameStr(interfaceName)
		if !exist || node.Kind != ast.NodeKindInterfaceTypeDefinition {
			continue
		}
		interfaceType := s.document.InterfaceTypeDefinitions[node.Ref]
		for _, fieldRef := range interfaceType.FieldsDefinition.Refs {
			if s.document.FieldDefinitionNameString(fieldRef) != fieldName {
				continue
			}
			_, skip := s.determineIfFieldWithFieldNameShouldBeSkipped(fieldRef, interfaceName, skippedFields...)
			if !s.typeFieldExists(interfaceName, fieldName, fieldArguments) && !skip {
				s.addTypeFieldArgsForFieldRef(fieldRef, interfaceName, fieldName, fieldArguments)
			}
		}
	}
}

// typeFieldExists checks if a type field combo exists in the arguments
func (s *Schema) typeFieldExists(typeName, fieldName string, fieldArguments *[]TypeFieldArguments) bool {
	for _, arg := range *fieldArguments {
		if arg.TypeName == typeName && arg.FieldName == fieldName {
			return true
		}
	}
	return false
}

func (s *Schema) addTypeFieldArgsForFieldRef(ref int, typeName string, fieldName string, fieldArguments *[]TypeFieldArguments) {
	currentTypeFieldArgs := TypeFieldArguments{
		TypeName:      typeName,
		FieldName:     fieldName,
		ArgumentNames: make([]string, 0),
	}

	for _, argRef := range s.document.FieldDefinitions[ref].ArgumentsDefinition.Refs {
		argName := s.document.InputValueDefinitionNameString(argRef)
		currentTypeFieldArgs.ArgumentNames = append(currentTypeFieldArgs.ArgumentNames, string(argName))
	}

	*fieldArguments = append(*fieldArguments, currentTypeFieldArgs)
}

func (s *Schema) GetAllNestedFieldChildrenFromTypeField(typeName string, fieldName string, skipFieldFuncs ...SkipFieldFunc) []TypeFields {
	node, fields := s.nodeFieldRefs(typeName)
	if len(fields) == 0 {
		return nil
	}
	childNodes := make([]TypeFields, 0)
	s.findInterfaceImplementations(node, &childNodes, skipFieldFuncs...)
	for _, ref := range fields {
		if fieldName == s.document.FieldDefinitionNameString(ref) {
			fieldTypeName := s.document.FieldDefinitionTypeNode(ref).NameString(&s.document)
			s.findNestedFieldChildren(fieldTypeName, &childNodes, skipFieldFuncs...)
			return childNodes
		}
	}

	return nil
}

func (s *Schema) findInterfaceImplementations(node ast.Node, childNodes *[]TypeFields, skipFieldFuncs ...SkipFieldFunc) {
	if node.Kind != ast.NodeKindInterfaceTypeDefinition {
		return
	}

	implementingNodes := s.document.InterfaceTypeDefinitionImplementedByRootNodes(node.Ref)
	for i := 0; i < len(implementingNodes); i++ {
		var typeName string
		switch implementingNodes[i].Kind {
		case ast.NodeKindObjectTypeDefinition:
			typeName = s.document.ObjectTypeDefinitionNameString(implementingNodes[i].Ref)
		case ast.NodeKindInterfaceTypeDefinition:
			typeName = s.document.InterfaceTypeDefinitionNameString(implementingNodes[i].Ref)
		}

		s.findNestedFieldChildren(typeName, childNodes, skipFieldFuncs...)
	}
}

func (s *Schema) findNestedFieldChildren(typeName string, childNodes *[]TypeFields, skipFieldFuncs ...SkipFieldFunc) {
	node, fields := s.nodeFieldRefs(typeName)
	if len(fields) == 0 {
		return
	}

	s.findInterfaceImplementations(node, childNodes, skipFieldFuncs...)
	for _, ref := range fields {
		fieldName := s.document.FieldDefinitionNameString(ref)
		if len(skipFieldFuncs) > 0 {
			skip := false
			for _, skipFieldFunc := range skipFieldFuncs {
				if skipFieldFunc != nil && skipFieldFunc(typeName, fieldName, s.document) {
					skip = true
					break
				}
			}

			if skip {
				continue
			}
		}

		if added := s.putChildNode(childNodes, typeName, fieldName); !added {
			continue
		}

		fieldTypeName := s.document.FieldDefinitionTypeNode(ref).NameString(&s.document)
		s.findNestedFieldChildren(fieldTypeName, childNodes, skipFieldFuncs...)
	}
}

func (s *Schema) nodeFieldRefs(typeName string) (node ast.Node, fieldsRefs []int) {
	node, exists := s.document.Index.FirstNodeByNameStr(typeName)
	if !exists {
		return ast.Node{}, nil
	}

	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		fieldsRefs = s.document.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs
	case ast.NodeKindInterfaceTypeDefinition:
		fieldsRefs = s.document.InterfaceTypeDefinitions[node.Ref].FieldsDefinition.Refs
	default:
		return ast.Node{}, nil
	}

	return node, fieldsRefs
}

func (s *Schema) putChildNode(nodes *[]TypeFields, typeName, fieldName string) (added bool) {
	for i := range *nodes {
		if typeName != (*nodes)[i].TypeName {
			continue
		}
		for j := range (*nodes)[i].FieldNames {
			if fieldName == (*nodes)[i].FieldNames[j] {
				return false
			}
		}
		(*nodes)[i].FieldNames = append((*nodes)[i].FieldNames, fieldName)
		return true
	}
	*nodes = append(*nodes, TypeFields{
		TypeName:   typeName,
		FieldNames: []string{fieldName},
	})
	return true
}

func createSchema(schemaContent []byte, mergeWithBaseSchema bool) (*Schema, error) {
	document, report := astparser.ParseGraphqlDocumentBytes(schemaContent)
	if report.HasErrors() {
		return nil, report
	}

	rawSchema := schemaContent
	if mergeWithBaseSchema {
		err := asttransform.MergeDefinitionWithBaseSchema(&document)
		if err != nil {
			return nil, err
		}

		rawSchemaBuffer := &bytes.Buffer{}
		err = astprinter.PrintIndent(&document, nil, []byte("  "), rawSchemaBuffer)
		if err != nil {
			return nil, err
		}

		rawSchema = rawSchemaBuffer.Bytes()
	}

	return &Schema{
		rawInput:  schemaContent,
		rawSchema: rawSchema,
		document:  document,
	}, nil
}

func SchemaIntrospection(schema *Schema) (*ExecutionResult, error) {
	var buf bytes.Buffer
	err := schema.IntrospectionResponse(&buf)
	return &ExecutionResult{&buf}, err
}

type SkipFieldFunc func(typeName, fieldName string, definition ast.Document) bool

func NewIsDataSourceConfigV2RootFieldSkipFunc(dataSources []plan.DataSourceConfiguration) SkipFieldFunc {
	return func(typeName, fieldName string, _ ast.Document) bool {
		for i := range dataSources {
			for j := range dataSources[i].RootNodes {
				if typeName != dataSources[i].RootNodes[j].TypeName {
					continue
				}
				for k := range dataSources[i].RootNodes[j].FieldNames {
					if fieldName == dataSources[i].RootNodes[j].FieldNames[k] {
						return true
					}
				}
			}
		}
		return false
	}
}

func NewSkipReservedNamesFunc() SkipFieldFunc {
	return func(typeName, fieldName string, _ ast.Document) bool {
		prefix := "__"
		return strings.HasPrefix(typeName, prefix) || strings.HasPrefix(fieldName, prefix)
	}
}
