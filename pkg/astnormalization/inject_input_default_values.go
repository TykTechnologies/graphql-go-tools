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

// TODO fix internal and external errors

func (in *inputFieldDefaultInjectionVisitor) EnterVariableDefinition(ref int) {
	in.variableName = in.operation.VariableDefinitionNameString(ref)
	in.variableTree = append(in.variableTree, in.variableName)

	if _, _, _, err := jsonparser.Get(in.operation.Input.Variables, in.variableName); err == jsonparser.KeyPathNotFoundError {
		in.StopWithInternalErr(errors.New("variable not defined in input"))
	} else if err != nil {
		in.StopWithInternalErr(err)
	}

	typeName := in.operation.BaseTypeNameBytes(ref)
	node, found := in.definition.Index.FirstNodeByNameBytes(typeName)
	if !found {
		in.StopWithInternalErr(errors.New("invalid variable type"))
	}
	if node.Kind != ast.NodeKindInputObjectTypeDefinition {
		return
	}

	// TODO check in nested input values
	inputDef := in.definition.InputObjectTypeDefinitions[node.Ref]
	for _, i := range inputDef.InputFieldsDefinition.Refs {
		valDef := in.definition.InputValueDefinitions[i]
		if in.definition.Types[valDef.Type].TypeKind != ast.TypeKindNonNull {
			continue
		}
		if !valDef.DefaultValue.IsDefined {
			continue
		}
		_, _, _, err := jsonparser.Get(in.operation.Input.Variables, in.variableName, in.definition.InputValueDefinitionNameString(i))
		if err == nil {
			return
		}
		if err != jsonparser.KeyPathNotFoundError {
			in.StopWithInternalErr(err)
		}
		defVal, err := in.definition.ValueToJSON(valDef.DefaultValue.Value)
		if err != nil {
			in.StopWithInternalErr(err)
		}

		newVariables, err := jsonparser.Set(in.operation.Input.Variables, defVal, append(in.variableTree, in.definition.InputValueDefinitionNameString(i))...)
		if err != nil {
			in.StopWithInternalErr(err)
		}
		in.operation.Input.Variables = newVariables

	}

	fmt.Println(node)
}

func (in *inputFieldDefaultInjectionVisitor) recursiveInjectInputFields(inputObjectRef int) {
	objectDef := in.definition.InputObjectTypeDefinitions[inputObjectRef]
	if !objectDef.HasInputFieldsDefinition {
		return
	}
	for _, i := range objectDef.InputFieldsDefinition.Refs {
		valDef := in.definition.InputValueDefinitions[i]
		if in.definition.Types[valDef.Type].TypeKind != ast.TypeKindNonNull {
			continue
		}
		if !valDef.DefaultValue.IsDefined {
			continue
		}
		_, _, _, err := jsonparser.Get(in.operation.Input.Variables, in.variableName, in.definition.InputValueDefinitionNameString(i))
		if err != nil && err != jsonparser.KeyPathNotFoundError {
			in.StopWithInternalErr(err)
		}
		if err == jsonparser.KeyPathNotFoundError {
			fmt.Println("not found")
		}
	}
}

func (in *inputFieldDefaultInjectionVisitor) LeaveVariableDefinition(ref int) {
	//TODO implement me
}
