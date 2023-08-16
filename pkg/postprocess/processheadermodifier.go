package postprocess

import (
	"bytes"
	"net/http"

	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
)

// HeaderModifier is a function type that modifies an http.Header.
type HeaderModifier func(header http.Header)

// ProcessHeaderModifier is a post-processor that modifies the FetchInput of an execution plan.
type ProcessHeaderModifier struct {
	modifier HeaderModifier
}

// NewProcessHeaderModifier creates a new ProcessHeaderModifier with the given modifier function.
func NewProcessHeaderModifier(modifier HeaderModifier) *ProcessHeaderModifier {
	return &ProcessHeaderModifier{modifier: modifier}
}

// Process traverses the execution plan and applies the modifier function to the FetchInput.
func (p *ProcessHeaderModifier) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.traverseNode(t.Response.Data)
	case *plan.StreamingResponsePlan:
		p.traverseNode(t.Response.InitialResponse.Data)
		for i := range t.Response.Patches {
			p.traverseFetch(t.Response.Patches[i].Fetch)
			p.traverseNode(t.Response.Patches[i].Value)
		}
	case *plan.SubscriptionResponsePlan:
		p.traverseTrigger(&t.Response.Trigger)
		p.traverseNode(t.Response.Response.Data)
	}
	return pre
}

// traverseNode traverses a resolve.Node and applies the modifier function to the FetchInput.
func (p *ProcessHeaderModifier) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		p.traverseFetch(n.Fetch)
		for i := range n.Fields {
			p.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		p.traverseNode(n.Item)
	}
}

// traverseFetch traverses a resolve.Fetch and applies the modifier function to the FetchInput.
func (p *ProcessHeaderModifier) traverseFetch(fetch resolve.Fetch) {
	if fetch == nil {
		return
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		p.traverseSingleFetch(f)
	case *resolve.BatchFetch:
		p.traverseSingleFetch(f.Fetch)
	case *resolve.ParallelFetch:
		for i := range f.Fetches {
			p.traverseFetch(f.Fetches[i])
		}
	}
}

// traverseTrigger applies the modifier function to the FetchInput of a resolve.GraphQLSubscriptionTrigger.
func (p *ProcessHeaderModifier) traverseTrigger(trigger *resolve.GraphQLSubscriptionTrigger) {
	header, _ := http.ReadRequest(bufio.NewReader(bytes.NewReader(trigger.Input)))
	modifiedHeader := p.modifyHeader(header)
	buf := new(bytes.Buffer)
	buf.Write(modifiedHeader)
	trigger.Input = buf.Bytes()
}

// traverseSingleFetch applies the modifier function to the FetchInput of a resolve.SingleFetch.
func (p *ProcessHeaderModifier) traverseSingleFetch(fetch *resolve.SingleFetch) {
	header, _ := http.ReadRequest(bufio.NewReader(bytes.NewReader(fetch.Input)))
	modifiedHeader := p.modifyHeader(header)
	buf := new(bytes.Buffer)
	buf.Write(modifiedHeader)
	fetch.Input = buf.Bytes()
}

// modifyHeader applies the modifier function to an http.Header and returns the modified header.
func (p *ProcessHeaderModifier) modifyHeader(input http.Header) http.Header {
	p.modifier(input)
	return input
}

