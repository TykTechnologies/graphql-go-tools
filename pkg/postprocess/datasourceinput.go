package postprocess

import (
	"errors"
	"fmt"
	"github.com/buger/jsonparser"
	"strconv"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
)

type ProcessDataSource struct{}

func (d *ProcessDataSource) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		d.traverseNode(t.Response.Data)
	case *plan.StreamingResponsePlan:
		d.traverseNode(t.Response.InitialResponse.Data)
		for i := range t.Response.Patches {
			d.traverseFetch(t.Response.Patches[i].Fetch)
			d.traverseNode(t.Response.Patches[i].Value)
		}
	case *plan.SubscriptionResponsePlan:
		d.traverseTrigger(&t.Response.Trigger)
		d.traverseNode(t.Response.Response.Data)
	}
	return pre
}

func (d *ProcessDataSource) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		d.traverseFetch(n.Fetch)
		for i := range n.Fields {
			d.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		d.traverseNode(n.Item)
	}
}

func (d *ProcessDataSource) traverseFetch(fetch resolve.Fetch) {
	if fetch == nil {
		return
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		d.traverseSingleFetch(f)
	case *resolve.BatchFetch:
		d.traverseSingleFetch(f.Fetch)
	case *resolve.ParallelFetch:
		for i := range f.Fetches {
			d.traverseFetch(f.Fetches[i])
		}
	}
}

func (d *ProcessDataSource) traverseTrigger(trigger *resolve.GraphQLSubscriptionTrigger) {
	d.resolveInputTemplate(trigger.Variables, string(trigger.Input), &trigger.InputTemplate)
	trigger.Input = nil
	trigger.Variables = nil
}

func (d *ProcessDataSource) traverseSingleFetch(fetch *resolve.SingleFetch) {
	d.resolveInputTemplate(fetch.Variables, fetch.Input, &fetch.InputTemplate)
	fetch.Input = ""
	fetch.Variables = nil
	fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull = fetch.SetTemplateOutputToNullOnVariableNull
	fetch.SetTemplateOutputToNullOnVariableNull = false
}

// correctGraphQLVariableTypes removes double quotes from the variable definition if the variable is not a string.
// This function is only intended for variables in a GraphQL request body.
func correctGraphQLVariableTypes(variables resolve.Variables, input string) string {
	// See TT-12313 for details.
	_, _, _, err := jsonparser.Get([]byte(input), "body", "variables")
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		// No variables, return the input as-is.
		return input
	}

	segments := strings.Split(input, "$$")
	isVariable := false
	for _, seg := range segments {
		switch {
		case isVariable:
			i, _ := strconv.Atoi(seg)
			variableTemplateSegment := (variables)[i].TemplateSegment()
			if variableTemplateSegment.Renderer == nil {
				continue
			}
			// Get the variable type from its renderer. If the type isn't a string, remove double quotes
			//
			// Possible types:
			// 	* NotExist
			//	* String
			//	* Number
			//	* Object
			//	* Array
			//	* Boolean
			//	* Null
			//	* Unknown
			if variableTemplateSegment.Renderer.GetRootValueType().Value != jsonparser.String {
				newVariable := fmt.Sprintf("$$%s$$", seg)
				oldVariable := fmt.Sprintf("\"%s\"", newVariable)
				input = strings.Replace(input, oldVariable, newVariable, 1)
			}
			isVariable = false
		default:
			isVariable = true
		}
	}
	return input
}

func (d *ProcessDataSource) resolveInputTemplate(variables resolve.Variables, input string, template *resolve.InputTemplate) {
	if input == "" {
		return
	}

	if !strings.Contains(input, "$$") {
		template.Segments = append(template.Segments, resolve.TemplateSegment{
			SegmentType: resolve.StaticSegmentType,
			Data:        []byte(input),
		})
		return
	}

	input = correctGraphQLVariableTypes(variables, input)
	segments := strings.Split(input, "$$")

	isVariable := false
	for _, seg := range segments {
		switch {
		case isVariable:
			i, _ := strconv.Atoi(seg)
			variableTemplateSegment := (variables)[i].TemplateSegment()
			template.Segments = append(template.Segments, variableTemplateSegment)
			isVariable = false
		default:
			template.Segments = append(template.Segments, resolve.TemplateSegment{
				SegmentType: resolve.StaticSegmentType,
				Data:        []byte(seg),
			})
			isVariable = true
		}
	}
}
