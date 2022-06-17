package astnormalization

import (
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

	exists, err := v.variableKeyExists(v.variableName)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
	if !exists {
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

	v.recursiveInjectInputFields(node.Ref, v.variableName)
}

func (v *inputFieldDefaultInjectionVisitor) recursiveInjectInputFields(inputObjectRef int, fieldName string) {
	v.jsonPath = append(v.jsonPath, fieldName)
	objectDef := v.definition.InputObjectTypeDefinitions[inputObjectRef]
	if !objectDef.HasInputFieldsDefinition {
		return
	}
	for _, i := range objectDef.InputFieldsDefinition.Refs {
		valDef := v.definition.InputValueDefinitions[i]
		keys := append(v.jsonPath, v.definition.InputValueDefinitionNameString(i))
		isTypeScalarOrEnum := v.definition.TypeIsScalar(valDef.Type, v.definition) || v.definition.TypeIsEnum(valDef.Type, v.definition)
		exists, err := v.variableKeyExists(keys...)
		if err != nil {
			v.StopWithInternalErr(err)
			return
		}
		if exists && !isTypeScalarOrEnum {
			if node, found := v.definition.Index.FirstNodeByNameBytes(v.definition.ResolveTypeNameBytes(valDef.Type)); found {
				v.recursiveInjectInputFields(node.Ref, keys[len(keys)-1])
			} else {
				continue
			}
		}
		if !valDef.DefaultValue.IsDefined {
			continue
		}
		defVal, err := v.definition.ValueToJSON(valDef.DefaultValue.Value)
		if err != nil {
			v.StopWithInternalErr(err)
			return
		}

		newVariables, err := jsonparser.Set(v.operation.Input.Variables, defVal, keys...)
		if err != nil {
			v.StopWithInternalErr(err)
			return
		}
		v.operation.Input.Variables = newVariables
	}
	v.jsonPath = v.jsonPath[:len(v.jsonPath)-1]
}

func (v *inputFieldDefaultInjectionVisitor) variableKeyExists(keys ...string) (exists bool, retErr error) {
	_, _, _, err := jsonparser.Get(v.operation.Input.Variables, keys...)
	switch err {
	case jsonparser.KeyPathNotFoundError:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

func (v *inputFieldDefaultInjectionVisitor) LeaveVariableDefinition(ref int) {
	v.variableName = ""
	v.jsonPath = make([]string, 0)
}
