package astnormalization

import (
	"errors"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func injectInputFieldDefaults(walker *astvisitor.Walker) *inputFieldDefaultInjectionVisitor {
	visitor := &inputFieldDefaultInjectionVisitor{
		Walker:   walker,
		jsonPath: make([]string, 0),
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterVariableDefinitionVisitor(visitor)
	return visitor
}

type inputFieldDefaultInjectionVisitor struct {
	*astvisitor.Walker

	operation  *ast.Document
	definition *ast.Document

	variableName string
	jsonPath     []string
}

func (v *inputFieldDefaultInjectionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
}

func (v *inputFieldDefaultInjectionVisitor) EnterVariableDefinition(ref int) {
	v.variableName = v.operation.VariableDefinitionNameString(ref)

	variableVal, _, _, err := jsonparser.Get(v.operation.Input.Variables, v.variableName)
	if err == jsonparser.KeyPathNotFoundError {
		return
	}
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}

	typeName := v.operation.ResolveTypeNameBytes(ref)
	node, found := v.definition.Index.FirstNodeByNameBytes(typeName)
	if !found {
		return
	}
	if node.Kind != ast.NodeKindInputObjectTypeDefinition {
		return
	}

	newVal, err := v.recursiveInjectInputFields(node.Ref, variableVal)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}

	newVariables, err := jsonparser.Set(v.operation.Input.Variables, newVal, v.variableName)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
	v.operation.Input.Variables = newVariables
}

func (v *inputFieldDefaultInjectionVisitor) recursiveInjectInputFields(inputObjectRef int, varValue []byte) ([]byte, error) {
	finalVal := varValue
	objectDef := v.definition.InputObjectTypeDefinitions[inputObjectRef]
	if !objectDef.HasInputFieldsDefinition {
		return varValue, nil
	}
	for _, i := range objectDef.InputFieldsDefinition.Refs {
		valDef := v.definition.InputValueDefinitions[i]
		fieldName := v.definition.InputValueDefinitionNameString(i)
		isTypeScalarOrEnum := v.definition.TypeIsScalar(valDef.Type, v.definition) || v.definition.TypeIsEnum(valDef.Type, v.definition)
		hasDefault := valDef.DefaultValue.IsDefined

		varVal, _, _, err := jsonparser.Get(varValue, fieldName)
		if err != nil && err != jsonparser.KeyPathNotFoundError {
			v.StopWithInternalErr(err)
			return nil, err
		}
		existsInVal := err != jsonparser.KeyPathNotFoundError

		if !isTypeScalarOrEnum {
			var valToUse []byte
			if existsInVal {
				valToUse = varVal
			} else if hasDefault {
				defVal, err := v.definition.ValueToJSON(valDef.DefaultValue.Value)
				if err != nil {
					return nil, err
				}
				valToUse = defVal
			} else {
				continue
			}
			fieldValue, err := v.processNonScalarField(valDef.Type, valToUse)
			if err != nil {
				return nil, err
			}
			finalVal, err = jsonparser.Set(finalVal, fieldValue, fieldName)
			if err != nil {
				return nil, err
			}
			continue
		}

		if !hasDefault && isTypeScalarOrEnum {
			continue
		}
		if existsInVal {
			continue
		}
		defVal, err := v.definition.ValueToJSON(valDef.DefaultValue.Value)
		if err != nil {
			return nil, err
		}

		finalVal, err = jsonparser.Set(finalVal, defVal, fieldName)
		if err != nil {
			return nil, err
		}
	}
	return finalVal, nil
}

func (v *inputFieldDefaultInjectionVisitor) processNonScalarField(fieldType int, defaultValue []byte) ([]byte, error) {
	finalVal := defaultValue
	fieldIsList := v.definition.TypeIsList(fieldType)
	varVal, valType, _, err := jsonparser.Get(defaultValue)
	if err != nil {
		return nil, err

	}
	node, found := v.definition.Index.FirstNodeByNameBytes(v.definition.ResolveTypeNameBytes(fieldType))
	if !found {
		return nil, nil
	}
	valIsList := valType == jsonparser.Array
	if fieldIsList && valIsList {
		// check if is nested list
		listOfList := v.definition.TypeIsList(v.definition.Types[fieldType].OfType)
		i := 0
		_, err := jsonparser.ArrayEach(varVal, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			if err != nil {
				return
			}
			if listOfList && dataType == jsonparser.Array {
				newVal, err := v.processNonScalarField(v.definition.Types[fieldType].OfType, value)
				if err != nil {
					return
				}
				finalVal, err = jsonparser.Set(defaultValue, newVal, fmt.Sprintf("[%d]", i))
				if err != nil {
					return
				}
			}
			if dataType == jsonparser.Object {
				newVal, err := v.recursiveInjectInputFields(node.Ref, value)
				if err != nil {
					return
				}
				finalVal, err = jsonparser.Set(defaultValue, newVal, fmt.Sprintf("[%d]", i))
				if err != nil {
					return
				}
			}
			i++
		})
		if err != nil {
			return nil, nil
		}
	} else if !fieldIsList && !valIsList {
		finalVal, err = v.recursiveInjectInputFields(node.Ref, defaultValue)
		if err != nil {
			return nil, nil
		}
	} else {
		return nil, errors.New("mismatched input value")
	}
	return finalVal, nil
}

func (v *inputFieldDefaultInjectionVisitor) calculateNestingDepth(typeRef int) int {
	var nestingDepth int
	for typeRef != ast.InvalidRef {
		first := v.definition.Types[typeRef]

		typeRef = first.OfType

		switch first.TypeKind {
		case ast.TypeKindList:
			nestingDepth++
		default:
			continue
		}
	}
	return nestingDepth
}

func (v *inputFieldDefaultInjectionVisitor) LeaveVariableDefinition(ref int) {
	v.variableName = ""
	v.jsonPath = make([]string, 0)
}
