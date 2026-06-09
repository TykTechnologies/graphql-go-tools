package postprocess

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/buger/jsonparser"

	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
)

type contextKey string

const headerModifierContextKey contextKey = "graphqlHeaderModifier"

func SetHeaderModifier(ctx context.Context, modifier HeaderModifier) context.Context {
	return context.WithValue(ctx, headerModifierContextKey, modifier)
}

func GetHeaderModifier(ctx context.Context) HeaderModifier {
	if modifier, ok := ctx.Value(headerModifierContextKey).(HeaderModifier); ok {
		return modifier
	}
	return nil
}

type HeaderModifier func(header http.Header)

type ProcessModifyHeader struct {
	headerModifier      HeaderModifier
	fetchInputProcessor *ProcessFetchInput
}

func NewProcessModifyHeader(headerModifier HeaderModifier) *ProcessModifyHeader {
	p := &ProcessModifyHeader{
		headerModifier: headerModifier,
	}
	p.fetchInputProcessor = NewProcessFetchInput(p.modifyHeader)
	return p
}

func (p *ProcessModifyHeader) Process(pre plan.Plan) plan.Plan {
	return p.fetchInputProcessor.Process(pre)
}

func (p *ProcessModifyHeader) modifyHeader(input []byte) string {
	if p.headerModifier == nil {
		return string(input)
	}

	var header http.Header
	val, valType, _, err := jsonparser.Get(input, "header")
	if err != nil && valType != jsonparser.NotExist {
		return string(input)
	}

	switch valType {
	case jsonparser.NotExist:
		header = make(http.Header)
	case jsonparser.Object:
		err := json.Unmarshal(val, &header)
		if err != nil {
			return string(input)
		}
	default:
		return string(input)
	}

	p.headerModifier(header)

	m, err := json.Marshal(header)
	if err != nil {
		return string(input)
	}
	updated, err := jsonparser.Set(input, m, "header")
	if err != nil {
		return string(input)
	}
	return string(updated)
}
