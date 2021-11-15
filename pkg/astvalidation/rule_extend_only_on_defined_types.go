package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func ExtendOnlyOnDefinedTypes() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := &extendOnlyOnDefinedTypesVisitor{
			Walker: walker,
		}

		walker.RegisterEnterDocumentVisitor(visitor)
		walker.RegisterEnterScalarTypeExtensionVisitor(visitor)
		walker.RegisterEnterObjectTypeExtensionVisitor(visitor)
		walker.RegisterEnterInterfaceTypeExtensionVisitor(visitor)
		walker.RegisterEnterUnionTypeExtensionVisitor(visitor)
		walker.RegisterEnterEnumTypeExtensionVisitor(visitor)
		walker.RegisterEnterInputObjectTypeExtensionVisitor(visitor)
	}
}

type extendOnlyOnDefinedTypesVisitor struct {
	*astvisitor.Walker
	definition *ast.Document
}

func (e *extendOnlyOnDefinedTypesVisitor) EnterDocument(operation, definition *ast.Document) {
	e.definition = operation
}

func (e *extendOnlyOnDefinedTypesVisitor) EnterScalarTypeExtension(ref int) {
	name := e.definition.ScalarTypeExtensionNameBytes(ref)
	if !e.extensionIsValidForNodeKind(name, ast.NodeKindScalarTypeDefinition) {
		e.Report.AddExternalError(operationreport.ErrScalarTypeUndefined(name))
	}
}

func (e *extendOnlyOnDefinedTypesVisitor) EnterObjectTypeExtension(ref int) {
	name := e.definition.ObjectTypeExtensionNameBytes(ref)
	if !e.extensionIsValidForNodeKind(name, ast.NodeKindObjectTypeDefinition) {
		e.Report.AddExternalError(operationreport.ErrTypeUndefined(name))
	}
}

func (e *extendOnlyOnDefinedTypesVisitor) EnterInterfaceTypeExtension(ref int) {
	name := e.definition.InterfaceTypeExtensionNameBytes(ref)
	if !e.extensionIsValidForNodeKind(name, ast.NodeKindInterfaceTypeDefinition) {
		e.Report.AddExternalError(operationreport.ErrInterfaceTypeUndefined(name))
	}
}

func (e *extendOnlyOnDefinedTypesVisitor) EnterUnionTypeExtension(ref int) {
	name := e.definition.UnionTypeExtensionNameBytes(ref)
	if !e.extensionIsValidForNodeKind(name, ast.NodeKindUnionTypeDefinition) {
		e.Report.AddExternalError(operationreport.ErrUnionTypeUndefined(name))
	}
}

func (e *extendOnlyOnDefinedTypesVisitor) EnterEnumTypeExtension(ref int) {
	name := e.definition.EnumTypeExtensionNameBytes(ref)
	if !e.extensionIsValidForNodeKind(name, ast.NodeKindEnumTypeDefinition) {
		e.Report.AddExternalError(operationreport.ErrEnumTypeUndefined(name))
	}
}

func (e *extendOnlyOnDefinedTypesVisitor) EnterInputObjectTypeExtension(ref int) {
	name := e.definition.InputObjectTypeExtensionNameBytes(ref)
	if !e.extensionIsValidForNodeKind(name, ast.NodeKindInputObjectTypeDefinition) {
		e.Report.AddExternalError(operationreport.ErrInputObjectTypeUndefined(name))
	}
}

func (e *extendOnlyOnDefinedTypesVisitor) extensionIsValidForNodeKind(name ast.ByteSlice, definitionNodeKind ast.NodeKind) bool {
	nodes, exists := e.definition.Index.NodesByNameBytes(name)
	if !exists {
		return true
	}

	for i := 0; i < len(nodes); i++ {
		if nodes[i].Kind == definitionNodeKind {
			return true
		}
	}

	return false
}
