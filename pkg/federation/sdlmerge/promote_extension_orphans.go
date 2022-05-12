package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type promoteExtensionOrphansVisitor struct {
	*astvisitor.Walker
	document              *ast.Document
	extensionSet          map[string]bool
	rootNodesToRemove     []ast.Node
	lastUnionExtensionRef int
}

func newPromoteExtensionOrphansVisitor() *promoteExtensionOrphansVisitor {
	return &promoteExtensionOrphansVisitor{
		nil,
		nil,
		make(map[string]bool, 0),
		nil,
		ast.InvalidRef,
	}
}

func (p *promoteExtensionOrphansVisitor) Register(walker *astvisitor.Walker) {
	p.Walker = walker
	walker.RegisterEnterDocumentVisitor(p)
	walker.RegisterEnterUnionTypeExtensionVisitor(p)
	walker.RegisterLeaveDocumentVisitor(p)
}

func (p *promoteExtensionOrphansVisitor) EnterDocument(operation, _ *ast.Document) {
	p.document = operation
}

func (p *promoteExtensionOrphansVisitor) EnterUnionTypeExtension(ref int) {
	if ref <= p.lastUnionExtensionRef {
		return
	}
	name := p.document.UnionTypeExtensionNameString(ref)
	if p.extensionSet[name] {
		p.Walker.StopWithExternalErr(operationreport.ErrUnresolvedExtensionOrphansMustBeUnique(name))
	}
	p.extensionSet[name] = true
	p.document.ImportAndExtendUnionTypeDefinitionByUnionTypeExtension(ref)
	p.rootNodesToRemove = append(p.rootNodesToRemove, ast.Node{Kind: ast.NodeKindUnionTypeExtension, Ref: ref})
	p.lastUnionExtensionRef = ref
}

func (p *promoteExtensionOrphansVisitor) LeaveDocument(_, _ *ast.Document) {
	if p.rootNodesToRemove != nil {
		p.document.DeleteRootNodes(p.rootNodesToRemove)
	}
}
