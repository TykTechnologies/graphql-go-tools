package sdlmerge

import (
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

func newExtendScalarTypeDefinition() *extendScalarTypeDefinitionVisitor {
	return &extendScalarTypeDefinitionVisitor{}
}

type extendScalarTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document *ast.Document
}

func (e *extendScalarTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterScalarTypeExtensionVisitor(e)
}

func (e *extendScalarTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendScalarTypeDefinitionVisitor) EnterScalarTypeExtension(ref int) {
	nodes, exists := e.document.Index.NodesByNameBytes(e.document.ScalarTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindScalarTypeDefinition {
			continue
		}
		if hasExtended {
			e.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.document.ScalarTypeExtensionNameString(ref)))
			return
		}
		e.document.ExtendScalarTypeDefinitionByScalarTypeExtension(nodes[i].Ref, ref)
		hasExtended = true
	}
	if !hasExtended {
		e.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.ScalarTypeExtensionNameBytes(ref)))
	}
}
