package resolve

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
)

// fetchTraces collects the traces recorded while one request is resolved. They must not be hung off
// the fetches themselves: a plan is shared by every concurrent request that resolves the same
// operation, so writing to it is a data race. Parallel fetches record concurrently within a single
// request, hence the mutex. The zero value is ready to use and all methods tolerate a nil receiver.
type fetchTraces struct {
	mu sync.Mutex
	// loads holds the trace of every fetch that was loaded, keyed by the fetch it belongs to.
	loads map[Fetch]*DataSourceLoadTrace
	// listItemFetches holds the per-item copies a ParallelListItemFetch was resolved with.
	listItemFetches map[*ParallelListItemFetch][]*SingleFetch
}

func (t *fetchTraces) reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loads = nil
	t.listItemFetches = nil
}

func (t *fetchTraces) setLoad(fetch Fetch, trace *DataSourceLoadTrace) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.loads == nil {
		t.loads = make(map[Fetch]*DataSourceLoadTrace)
	}
	t.loads[fetch] = trace
}

func (t *fetchTraces) load(fetch Fetch) *DataSourceLoadTrace {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.loads[fetch]
}

func (t *fetchTraces) setListItemFetches(fetch *ParallelListItemFetch, fetches []*SingleFetch) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.listItemFetches == nil {
		t.listItemFetches = make(map[*ParallelListItemFetch][]*SingleFetch)
	}
	t.listItemFetches[fetch] = fetches
}

func (t *fetchTraces) listItemFetchesOf(fetch *ParallelListItemFetch) []*SingleFetch {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.listItemFetches[fetch]
}

type RequestTraceOptions struct {
	// Enable switches tracing on or off
	Enable bool
	// ExcludePlannerStats excludes planner timing information from the trace output
	ExcludePlannerStats bool
	// ExcludeRawInputData excludes the raw input for a load operation from the trace output
	ExcludeRawInputData bool
	// ExcludeInput excludes the rendered input for a load operation from the trace output
	ExcludeInput bool
	// ExcludeOutput excludes the result of a load operation from the trace output
	ExcludeOutput bool
	// ExcludeLoadStats excludes the load timing information from the trace output
	ExcludeLoadStats bool
	// EnablePredictableDebugTimings makes the timings in the trace output predictable for debugging purposes
	EnablePredictableDebugTimings          bool
	IncludeTraceOutputInResponseExtensions bool
}

func (r *RequestTraceOptions) EnableAll() {
	r.Enable = true
	r.ExcludePlannerStats = false
	r.ExcludeRawInputData = false
	r.ExcludeInput = false
	r.ExcludeOutput = false
	r.ExcludeLoadStats = false
	r.EnablePredictableDebugTimings = false
	r.IncludeTraceOutputInResponseExtensions = true
}

func (r *RequestTraceOptions) DisableAll() {
	r.Enable = false
	r.ExcludePlannerStats = true
	r.ExcludeRawInputData = true
	r.ExcludeInput = true
	r.ExcludeOutput = true
	r.ExcludeLoadStats = true
	r.EnablePredictableDebugTimings = false
	r.IncludeTraceOutputInResponseExtensions = false
}

type TraceFetchType string

const (
	TraceFetchTypeSingle           TraceFetchType = "single"
	TraceFetchTypeParallel         TraceFetchType = "parallel"
	TraceFetchTypeSerial           TraceFetchType = "serial"
	TraceFetchTypeParallelListItem TraceFetchType = "parallelListItem"
	TraceFetchTypeEntity           TraceFetchType = "entity"
	TraceFetchTypeBatchEntity      TraceFetchType = "batchEntity"
)

type TraceNodeType string

const (
	TraceNodeTypeObject      TraceNodeType = "object"
	TraceNodeTypeEmptyObject TraceNodeType = "emptyObject"
	TraceNodeTypeArray       TraceNodeType = "array"
	TraceNodeTypeEmptyArray  TraceNodeType = "emptyArray"
	TraceNodeTypeNull        TraceNodeType = "null"
	TraceNodeTypeString      TraceNodeType = "string"
	TraceNodeTypeBoolean     TraceNodeType = "boolean"
	TraceNodeTypeInteger     TraceNodeType = "integer"
	TraceNodeTypeFloat       TraceNodeType = "float"
	TraceNodeTypeBigInt      TraceNodeType = "bigint"
	TraceNodeTypeCustom      TraceNodeType = "custom"
	TraceNodeTypeScalar      TraceNodeType = "scalar"
	TraceNodeTypeUnknown     TraceNodeType = "unknown"
)

type TraceFetch struct {
	Id                   string                 `json:"id,omitempty"`
	Type                 TraceFetchType         `json:"type,omitempty"`
	Path                 string                 `json:"path,omitempty"`
	DataSourceID         string                 `json:"data_source_id,omitempty"`
	Fetches              []*TraceFetch          `json:"fetches,omitempty"`
	DataSourceLoadTrace  *DataSourceLoadTrace   `json:"datasource_load_trace,omitempty"`
	DataSourceLoadTraces []*DataSourceLoadTrace `json:"data_source_load_traces,omitempty"`
}

type TraceFetchEvents struct {
	InputBeforeSourceLoad json.RawMessage `json:"input_before_source_load,omitempty"`
}

type TraceField struct {
	Name            string     `json:"name,omitempty"`
	Value           *TraceNode `json:"value,omitempty"`
	ParentTypeNames []string   `json:"parent_type_names,omitempty"`
	NamedType       string     `json:"named_type,omitempty"`
	DataSourceIDs   []string   `json:"data_source_ids,omitempty"`
}

type TraceNode struct {
	Info                 *TraceInfo    `json:"info,omitempty"`
	Fetch                *TraceFetch   `json:"fetch,omitempty"`
	NodeType             TraceNodeType `json:"node_type,omitempty"`
	Nullable             bool          `json:"nullable,omitempty"`
	Path                 []string      `json:"path,omitempty"`
	Fields               []*TraceField `json:"fields,omitempty"`
	Items                []*TraceNode  `json:"items,omitempty"`
	UnescapeResponseJson bool          `json:"unescape_response_json,omitempty"`
	IsTypeName           bool          `json:"is_type_name,omitempty"`
}

func getNodeType(kind NodeKind) TraceNodeType {
	switch kind {
	case NodeKindObject:
		return TraceNodeTypeObject
	case NodeKindEmptyObject:
		return TraceNodeTypeEmptyObject
	case NodeKindArray:
		return TraceNodeTypeArray
	case NodeKindEmptyArray:
		return TraceNodeTypeEmptyArray
	case NodeKindNull:
		return TraceNodeTypeNull
	case NodeKindString:
		return TraceNodeTypeString
	case NodeKindBoolean:
		return TraceNodeTypeBoolean
	case NodeKindInteger:
		return TraceNodeTypeInteger
	case NodeKindFloat:
		return TraceNodeTypeFloat
	case NodeKindBigInt:
		return TraceNodeTypeBigInt
	case NodeKindCustom:
		return TraceNodeTypeCustom
	case NodeKindScalar:
		return TraceNodeTypeScalar
	default:
		return TraceNodeTypeUnknown
	}
}

func parseField(f *Field, traces *fetchTraces) *TraceField {
	if f == nil {
		return nil
	}

	field := &TraceField{
		Name:  string(f.Name),
		Value: parseNode(f.Value, traces),
	}

	if f.Info == nil {
		return field
	}

	field.ParentTypeNames = f.Info.ParentTypeNames
	field.NamedType = f.Info.NamedType
	field.DataSourceIDs = f.Info.Source.IDs

	return field
}

func parseFetch(fetch Fetch, traces *fetchTraces) *TraceFetch {
	traceFetch := &TraceFetch{
		Id: uuid.NewString(),
	}

	switch f := fetch.(type) {
	case *SingleFetch:
		traceFetch.Type = TraceFetchTypeSingle
		if trace := traces.load(f); trace != nil {
			traceFetch.DataSourceLoadTrace = trace
			traceFetch.Path = trace.Path
		}
		if f.Info != nil {
			traceFetch.DataSourceID = f.Info.DataSourceID
		}

	case *ParallelFetch:
		traceFetch.Type = TraceFetchTypeParallel
		if trace := traces.load(f); trace != nil {
			traceFetch.Path = trace.Path
		}
		for _, subFetch := range f.Fetches {
			traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(subFetch, traces))
		}

	case *SerialFetch:
		traceFetch.Type = TraceFetchTypeSerial
		if trace := traces.load(f); trace != nil {
			traceFetch.Path = trace.Path
		}
		for _, subFetch := range f.Fetches {
			traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(subFetch, traces))
		}

	case *ParallelListItemFetch:
		traceFetch.Type = TraceFetchTypeParallelListItem
		if trace := traces.load(f); trace != nil {
			traceFetch.Path = trace.Path
		}
		traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(f.Fetch, traces))
		for _, itemFetch := range traces.listItemFetchesOf(f) {
			if trace := traces.load(itemFetch); trace != nil {
				traceFetch.DataSourceLoadTraces = append(traceFetch.DataSourceLoadTraces, trace)
			}
			if itemFetch.Info != nil {
				traceFetch.DataSourceID = itemFetch.Info.DataSourceID
			}
		}
	case *EntityFetch:
		traceFetch.Type = TraceFetchTypeEntity
		if trace := traces.load(f); trace != nil {
			traceFetch.DataSourceLoadTrace = trace
			traceFetch.Path = trace.Path
		}
		if f.Info != nil {
			traceFetch.DataSourceID = f.Info.DataSourceID
		}

	case *BatchEntityFetch:
		traceFetch.Type = TraceFetchTypeBatchEntity
		if trace := traces.load(f); trace != nil {
			traceFetch.DataSourceLoadTrace = trace
			traceFetch.Path = trace.Path
		}
		if f.Info != nil {
			traceFetch.DataSourceID = f.Info.DataSourceID
		}

	default:
		return nil
	}

	return traceFetch
}

func parseNode(n Node, traces *fetchTraces) *TraceNode {
	node := &TraceNode{
		NodeType: getNodeType(n.NodeKind()),
		Nullable: n.NodeKind() == NodeKindNull || n.NodePath() == nil,
		Path:     n.NodePath(),
	}

	switch v := n.(type) {
	case *Object:
		for _, field := range v.Fields {
			node.Fields = append(node.Fields, parseField(field, traces))
		}
		node.Fetch = parseFetch(v.Fetch, traces)

	case *Array:
		if v.Item != nil {
			node.Items = append(node.Items, parseNode(v.Item, traces))
		} else if len(v.Items) > 0 {
			for _, item := range v.Items {
				node.Items = append(node.Items, parseNode(item, traces))
			}
		}

	case *String:
		node.UnescapeResponseJson = v.UnescapeResponseJson
		node.IsTypeName = v.IsTypeName
	}

	return node
}

// GetTrace assembles the trace of a resolved response. It cannot report the per-fetch load traces,
// which belong to a single request rather than to the shared plan - resolving a response reports
// those itself, through Resolvable.
func GetTrace(ctx context.Context, root *Object) *TraceNode {
	return getTrace(ctx, root, nil)
}

func getTrace(ctx context.Context, root *Object, traces *fetchTraces) *TraceNode {
	node := parseNode(root, traces)
	node.Info = GetTraceInfo(ctx)
	return node
}
