package astnormalization

import (
	"bytes"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
)

// FragmentSpreadDepth is a helper for nested Fragments to calculate the actual depth of a Fragment Node
type FragmentSpreadDepth struct {
	walker             astvisitor.Walker
	visitor            fragmentSpreadDepthVisitor
	calc               nestedDepthCalc
	visitorsRegistered bool
}

// Depth holds all necessary information to understand the Depth of a Fragment Node
type Depth struct {
	SpreadRef          int
	Depth              int
	SpreadName         ast.ByteSlice
	isNested           bool
	parentFragmentName ast.ByteSlice
}

type Depths []Depth

func (d Depths) ByRef(ref int) (int, bool) {
	for i := range d {
		if d[i].SpreadRef == ref {
			return d[i].Depth, true
		}
	}
	return -1, false
}

// Get returns all FragmentSpread Depths for a given AST
func (r *FragmentSpreadDepth) Get(operation, definition *ast.Document, report *operationreport.Report, depths *Depths) {

	if !r.visitorsRegistered {
		r.walker.RegisterEnterFragmentSpreadVisitor(&r.visitor)
		r.visitorsRegistered = true
	}

	r.visitor.operation = operation
	r.visitor.definition = definition
	r.visitor.depths = depths
	r.visitor.Walker = &r.walker

	r.walker.Walk(operation, definition, report)
	r.calc.calculatedNestedDepths(depths, report)
}

type nestedDepthCalc struct {
	depths    *Depths
	report    *operationreport.Report
	resolving map[string]bool
}

func (n *nestedDepthCalc) calculatedNestedDepths(depths *Depths, report *operationreport.Report) {
	n.depths = depths
	n.report = report
	n.resolving = make(map[string]bool, len(*depths))

	for i := range *depths {
		(*depths)[i].Depth = n.calculateNestedDepth(i)
		if report.HasErrors() {
			return
		}
	}
}

func (n *nestedDepthCalc) calculateNestedDepth(i int) int {
	if !(*n.depths)[i].isNested {
		return (*n.depths)[i].Depth
	}
	return (*n.depths)[i].Depth + n.depthForFragment((*n.depths)[i].parentFragmentName)
}

// depthForFragment resolves the nested depth contributed by the fragment named name.
// A fragment can only be resolved once at a time on the current call path: if it is
// spread transitively from within itself (directly or through other fragments), that's
// a fragment cycle, which would otherwise recurse until the goroutine stack overflows.
func (n *nestedDepthCalc) depthForFragment(name ast.ByteSlice) int {
	key := string(name)
	if n.resolving[key] {
		n.report.AddExternalError(operationreport.ErrFragmentSpreadFormsCycle(name))
		return 0
	}

	for i := range *n.depths {
		if bytes.Equal(name, (*n.depths)[i].SpreadName) {
			n.resolving[key] = true
			depth := n.calculateNestedDepth(i)
			delete(n.resolving, key)
			return depth
		}
	}
	return 0
}

type fragmentSpreadDepthVisitor struct {
	*astvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
	depths     *Depths
}

func (r *fragmentSpreadDepthVisitor) EnterFragmentSpread(ref int) {

	depth := Depth{
		SpreadRef:  ref,
		Depth:      r.Depth,
		SpreadName: r.operation.FragmentSpreadNameBytes(ref),
	}

	if r.Ancestors[0].Kind == ast.NodeKindFragmentDefinition {
		depth.isNested = true
		depth.parentFragmentName = r.operation.FragmentDefinitionNameBytes(r.Ancestors[0].Ref)
	}

	*r.depths = append(*r.depths, depth)
}
