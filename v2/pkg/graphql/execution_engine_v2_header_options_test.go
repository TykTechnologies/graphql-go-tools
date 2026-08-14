package graphql

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/starwars"
)

// These tests verify that WithHeaderModifier/WithUpstreamHeaders are applied
// fresh on every Execute call - including when the underlying plan is served
// from the engine's plan cache - rather than only once at plan-build time.
// Each test calls Execute twice on the SAME engine with the SAME query but a
// DIFFERENT header option value, so the second call is guaranteed to hit the
// plan cache.

// newHeaderOptionsTestEngine builds a single ExecutionEngineV2 whose one
// upstream datasource has a baseline "Authorization" header configured (so
// WithHeaderModifier has an existing "header" JSON object to rewrite) and
// whose RoundTripper records every outgoing request's headers, in call order.
func newHeaderOptionsTestEngine(t *testing.T) (engine *ExecutionEngineV2, capturedHeaders *[]http.Header) {
	t.Helper()

	schema := starwarsSchema(t)
	capturedHeaders = &[]http.Header{}

	dataSources := []plan.DataSourceConfiguration{
		{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"hero"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Character", FieldNames: []string{"name"}},
			},
			Factory: &graphql_datasource.Factory{
				HTTPClient: testNetHttpClient(t, roundTripperTestCase{
					expectedHost:     "example.com",
					expectedPath:     "/",
					sendResponseBody: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
					sendStatusCode:   200,
					capturedHeaders:  capturedHeaders,
				}),
			},
			Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
				Fetch: graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
					Header: http.Header{"Authorization": []string{"placeholder"}},
				},
				UpstreamSchema: string(schema.Document()),
			}),
		},
	}

	engineConf := NewEngineV2Configuration(schema)
	engineConf.SetDataSources(dataSources)
	engineConf.SetFieldConfigurations([]plan.FieldConfiguration{})

	var err error
	engine, err = NewExecutionEngineV2(context.Background(), abstractlogger.Noop{}, engineConf)
	require.NoError(t, err)

	return engine, capturedHeaders
}

func heroRequest(t *testing.T) Request {
	return loadStarWarsQuery(starwars.FileSimpleHeroQuery, nil)(t)
}

func TestExecutionEngineV2_Execute_HeaderModifierAppliedPerRequest(t *testing.T) {
	engine, capturedHeaders := newHeaderOptionsTestEngine(t)
	ctx := context.Background()

	setAuthHeader := func(value string) postprocess.HeaderModifier {
		return func(header http.Header) { header.Set("Authorization", value) }
	}

	// First call: this is a plan-cache MISS, so it populates the cache.
	op1 := heroRequest(t)
	w1 := NewEngineResultWriter()
	err := engine.Execute(ctx, &op1, &w1, WithHeaderModifier(setAuthHeader("value-A")))
	require.NoError(t, err)
	require.Equal(t, `{"data":{"hero":{"name":"Luke Skywalker"}}}`, w1.String())

	// Second call: DIFFERENT header value, but the SAME query text -> plan-cache HIT.
	op2 := heroRequest(t)
	w2 := NewEngineResultWriter()
	err = engine.Execute(ctx, &op2, &w2, WithHeaderModifier(setAuthHeader("value-B")))
	require.NoError(t, err)
	require.Equal(t, `{"data":{"hero":{"name":"Luke Skywalker"}}}`, w2.String())

	// Sanity check: both calls really did hash to the same cached plan.
	require.Equal(t, 1, engine.executionPlanCache.Len(), "test setup invariant: both calls must hash to the same cached plan")

	require.Len(t, *capturedHeaders, 2)
	assert.Equal(t, "value-A", (*capturedHeaders)[0].Get("Authorization"))
	assert.Equal(t, "value-B", (*capturedHeaders)[1].Get("Authorization"),
		"the second call must use its own header value, not a value cached from the first call")
}

func TestExecutionEngineV2_Execute_UpstreamHeadersAppliedPerRequest(t *testing.T) {
	engine, capturedHeaders := newHeaderOptionsTestEngine(t)
	ctx := context.Background()

	authHeader := func(value string) http.Header {
		return http.Header{"Authorization": []string{value}}
	}

	// First call: this is a plan-cache MISS, so it populates the cache.
	op1 := heroRequest(t)
	w1 := NewEngineResultWriter()
	err := engine.Execute(ctx, &op1, &w1, WithUpstreamHeaders(authHeader("value-A")))
	require.NoError(t, err)
	require.Equal(t, `{"data":{"hero":{"name":"Luke Skywalker"}}}`, w1.String())

	// Second call: DIFFERENT header value, but the SAME query text -> plan-cache HIT.
	op2 := heroRequest(t)
	w2 := NewEngineResultWriter()
	err = engine.Execute(ctx, &op2, &w2, WithUpstreamHeaders(authHeader("value-B")))
	require.NoError(t, err)
	require.Equal(t, `{"data":{"hero":{"name":"Luke Skywalker"}}}`, w2.String())

	require.Equal(t, 1, engine.executionPlanCache.Len(), "test setup invariant: both calls must hash to the same cached plan")

	require.Len(t, *capturedHeaders, 2)
	assert.Equal(t, "value-A", (*capturedHeaders)[0].Get("Authorization"))
	assert.Equal(t, "value-B", (*capturedHeaders)[1].Get("Authorization"),
		"the second call must use its own header value, not a value cached from the first call")
}

func TestExecutionEngineV2_Execute_PlanCacheHeaderIsRequestScoped(t *testing.T) {
	schema := starwarsSchema(t)

	dataSources := []plan.DataSourceConfiguration{
		{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"hero"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Character", FieldNames: []string{"name"}},
			},
			Factory: &graphql_datasource.Factory{},
			Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
				Fetch: graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
					Header: http.Header{"Authorization": []string{"placeholder"}},
				},
				UpstreamSchema: string(schema.Document()),
			}),
		},
	}

	engineConf := NewEngineV2Configuration(schema)
	engineConf.SetDataSources(dataSources)
	engineConf.SetFieldConfigurations([]plan.FieldConfiguration{})

	engine, err := NewExecutionEngineV2(context.Background(), abstractlogger.Noop{}, engineConf)
	require.NoError(t, err)

	operation := heroRequest(t)
	require.NoError(t, engine.Normalize(&operation))

	execCtx := newInternalExecutionContext()
	WithHeaderModifier(func(header http.Header) {
		header.Set("Authorization", "value-A")
	})(execCtx.postProcessor, execCtx.resolveContext)

	var report operationreport.Report
	planResult, err := engine.Plan(execCtx.postProcessor, &operation, &report)
	require.NoError(t, err)
	require.False(t, report.HasErrors())
	require.Equal(t, 1, engine.executionPlanCache.Len())

	syncPlan, ok := planResult.(*plan.SynchronousResponsePlan)
	require.True(t, ok)
	singleFetch, ok := syncPlan.Response.Data.Fetch.(*resolve.SingleFetch)
	require.True(t, ok)

	renderedBuf := &bytes.Buffer{}
	renderCtx := resolve.NewContext(context.Background())
	err = singleFetch.InputTemplate.Render(renderCtx, nil, renderedBuf)
	require.NoError(t, err)

	assert.NotContains(t, renderedBuf.String(), "value-A",
		"the cached plan must not contain any per-request header value frozen into its static InputTemplate segments")
}
