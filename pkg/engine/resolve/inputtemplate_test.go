package resolve

import (
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/pkg/fastbuffer"
)

func TestInputTemplate_Render(t *testing.T) {
	runTest := func(t *testing.T, variables string, sourcePath []string, jsonSchema string, expectErr bool, expected string) {
		t.Helper()

		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: sourcePath,
					Renderer:           NewPlainVariableRendererWithValidation(jsonSchema),
				},
			},
		}
		ctx := &Context{
			Variables: []byte(variables),
		}
		buf := fastbuffer.New()
		err := template.Render(ctx, nil, buf)
		if expectErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		out := buf.String()
		assert.Equal(t, expected, out)
	}

	t.Run("string scalar", func(t *testing.T) {
		runTest(t, `{"foo":"bar"}`, []string{"foo"}, `{"type":"string"}`, false, `"bar"`)
	})
	t.Run("boolean scalar", func(t *testing.T) {
		runTest(t, `{"foo":true}`, []string{"foo"}, `{"type":"boolean"}`, false, "true")
	})
	t.Run("json object pass through", func(t *testing.T) {
		runTest(t, `{"foo":{"bar":"baz"}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"string"}}}`, false, `{"bar":"baz"}`)
	})
	t.Run("json object as graphql object", func(t *testing.T) {
		runTest(t, `{"foo":{"bar":"baz"}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"string"}}}`, false, `{"bar":"baz"}`)
	})
	t.Run("json object on non-required type as graphql object with null", func(t *testing.T) {
		runTest(t, `{"foo":null}`, []string{"foo"}, `{"type":["string","null"]}`, false, `null`)
	})
	t.Run("json object on required type as graphql object with null", func(t *testing.T) {
		runTest(t, `{"foo":null}`, []string{"foo"}, `{"type":"string"}`, true, ``)
	})
	t.Run("json object as graphql object with number", func(t *testing.T) {
		runTest(t, `{"foo":123}`, []string{"foo"}, `{"type":"integer"}`, false, `123`)
	})
	t.Run("json object as graphql object with invalid number", func(t *testing.T) {
		runTest(t, `{"foo":123}`, []string{"foo"}, `{"type":"string"}`, true, "")
	})
	t.Run("json object as graphql object with boolean", func(t *testing.T) {
		runTest(t, `{"foo":{"bar":true}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"boolean"}}}`, false, `{"bar":true}`)
	})
	t.Run("json object as graphql object with number", func(t *testing.T) {
		runTest(t, `{"foo":{"bar":123}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"integer"}}}`, false, `{"bar":123}`)
	})
	t.Run("json object as graphql object with float", func(t *testing.T) {
		runTest(t, `{"foo":{"bar":1.23}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"number"}}}`, false, `{"bar":1.23}`)
	})
	t.Run("json object as graphql object with nesting", func(t *testing.T) {
		runTest(t, `{"foo":{"bar":{"baz":"bat"}}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"object","properties":{"baz":{"type":"string"}}}}}`, false, `{"bar":{"baz":"bat"}}`)
	})
	t.Run("json object as graphql object with single array", func(t *testing.T) {
		runTest(t, `{"foo":["bar"]}`, []string{"foo"}, `{"type":"array","item":{"type":"string"}}`, false, `["bar"]`)
	})
	t.Run("json object as graphql object with array", func(t *testing.T) {
		runTest(t, `{"foo":["bar","baz"]}`, []string{"foo"}, `{"type":"array","item":{"type":"string"}}`, false, `["bar","baz"]`)
	})
	t.Run("json object as graphql object with object array", func(t *testing.T) {
		runTest(t, `{"foo":[{"bar":"baz"},{"bar":"bat"}]}`, []string{"foo"}, `{"type":"array","item":{"type":"object","properties":{"bar":{"type":"string"}}}}`, false, `[{"bar":"baz"},{"bar":"bat"}]`)
	})
	t.Run("array with csv render string", func(t *testing.T) {
		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: []string{"a"},
					Renderer:           NewCSVVariableRenderer(JsonRootType{Value: jsonparser.String, Kind: JsonRootTypeKindSingle}),
				},
			},
		}
		ctx := &Context{
			Variables: []byte(`{"a":["foo","bar"]}`),
		}
		buf := fastbuffer.New()
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, "foo,bar", out)
	})
	t.Run("array with csv render int", func(t *testing.T) {
		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: []string{"a"},
					Renderer:           NewCSVVariableRenderer(JsonRootType{Value: jsonparser.Number}),
				},
			},
		}
		ctx := &Context{
			Variables: []byte(`{"a":[1,2,3]}`),
		}
		buf := fastbuffer.New()
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, "1,2,3", out)
	})
	t.Run("array with default render int", func(t *testing.T) {
		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: []string{"a"},
					Renderer:           NewGraphQLVariableRenderer(`{"type":"array","items":{"type":"number"}}`),
				},
			},
		}
		ctx := &Context{
			Variables: []byte(`{"a":[1,2,3]}`),
		}
		buf := fastbuffer.New()
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, "[1,2,3]", out)
	})
	t.Run("json render with value missing", func(t *testing.T) {
		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType: StaticSegmentType,
					Data:        []byte(`{"key":`),
				},
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: []string{"a"},
					Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
				},
				{
					SegmentType: StaticSegmentType,
					Data:        []byte(`}`),
				},
			},
		}
		ctx := &Context{
			Variables: []byte(""),
		}
		buf := fastbuffer.New()
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, `{"key":null}`, out)
	})
}
