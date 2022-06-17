package astnormalization

import (
	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func injectInputFieldDefaults(walker *astvisitor.Walker) *inputFieldDefaultInjectionVisitor {
	visitor := &inputFieldDefaultInjectionVisitor{
		Walker:       walker,
		variableTree: make([]string, 0),
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
	variableTree []string
}

func (in *inputFieldDefaultInjectionVisitor) EnterDocument(operation, definition *ast.Document) {
	in.operation, in.definition = operation, definition
}

func (in *inputFieldDefaultInjectionVisitor) EnterVariableDefinition(ref int) {
	in.variableName = in.operation.VariableDefinitionNameString(ref)

	if _, _, _, err := jsonparser.Get(in.operation.Input.Variables, in.variableName); err == jsonparser.KeyPathNotFoundError {
		return
	} else if err != nil {
		in.StopWithInternalErr(err)
	}

	typeName := in.operation.BaseTypeNameBytes(ref)
	node, found := in.definition.Index.FirstNodeByNameBytes(typeName)
	if !found {
		return
	}
	if node.Kind != ast.NodeKindInputObjectTypeDefinition {
		return
	}

	in.recursiveInjectInputFields(node.Ref, in.variableName)
}

func (in *inputFieldDefaultInjectionVisitor) recursiveInjectInputFields(inputObjectRef int, fieldName string) {
	in.variableTree = append(in.variableTree, fieldName)
	objectDef := in.definition.InputObjectTypeDefinitions[inputObjectRef]
	if !objectDef.HasInputFieldsDefinition {
		return
	}
	for _, i := range objectDef.InputFieldsDefinition.Refs {
		valDef := in.definition.InputValueDefinitions[i]
		if in.definition.Types[valDef.Type].TypeKind != ast.TypeKindNonNull {
			continue
		}
		keys := append(in.variableTree, in.definition.InputValueDefinitionNameString(i))
		exists, err := in.variableKeyExists(keys...)
		typeScalarEnum := in.definition.TypeIsScalar(valDef.Type, in.definition) || in.definition.TypeIsEnum(valDef.Type, in.definition)
		if err != nil {
			in.StopWithInternalErr(err)
			return
		}
		if exists && !typeScalarEnum {
			if node, found := in.definition.Index.FirstNodeByNameBytes(in.definition.BaseTypeNameBytes(valDef.Type)); found {
				in.recursiveInjectInputFields(node.Ref, keys[len(keys)-1])
			} else {
				continue
			}
		}
		if !valDef.DefaultValue.IsDefined {
			continue
		}
		defVal, err := in.definition.ValueToJSON(valDef.DefaultValue.Value)
		if err != nil {
			in.StopWithInternalErr(err)
		}

		newVariables, err := jsonparser.Set(in.operation.Input.Variables, defVal, keys...)
		if err != nil {
			in.StopWithInternalErr(err)
		}
		in.operation.Input.Variables = newVariables
	}
	in.variableTree = in.variableTree[:len(in.variableTree)-1]
}

func (in *inputFieldDefaultInjectionVisitor) variableKeyExists(keys ...string) (exists bool, retErr error) {
	_, _, _, err := jsonparser.Get(in.operation.Input.Variables, keys...)
	switch err {
	case jsonparser.KeyPathNotFoundError:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

func (in *inputFieldDefaultInjectionVisitor) LeaveVariableDefinition(ref int) {
	in.variableName = ""
	in.variableTree = make([]string, 0)
}
