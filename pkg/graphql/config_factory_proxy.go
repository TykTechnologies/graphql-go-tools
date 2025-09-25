package graphql

import (
	"github.com/TykTechnologies/graphql-go-tools/pkg/customdirective"
	"net/http"
	"time"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astparser"
	graphqlDataSource "github.com/TykTechnologies/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
)

type proxyEngineConfigFactoryOptions struct {
	httpClient                *http.Client
	streamingClient           *http.Client
	subscriptionClientFactory graphqlDataSource.GraphQLSubscriptionClientFactory
	customDirectives          map[string]customdirective.CustomDirective
}

type ProxyEngineConfigFactoryOption func(options *proxyEngineConfigFactoryOptions)

func WithProxyCustomDirectives(customDirectives map[string]customdirective.CustomDirective) ProxyEngineConfigFactoryOption {
	return func(options *proxyEngineConfigFactoryOptions) {
		options.customDirectives = customDirectives
	}
}

func WithProxyHttpClient(client *http.Client) ProxyEngineConfigFactoryOption {
	return func(options *proxyEngineConfigFactoryOptions) {
		options.httpClient = client
	}
}

func WithProxyStreamingClient(client *http.Client) ProxyEngineConfigFactoryOption {
	return func(options *proxyEngineConfigFactoryOptions) {
		options.streamingClient = client
	}
}

func WithProxySubscriptionClientFactory(factory graphqlDataSource.GraphQLSubscriptionClientFactory) ProxyEngineConfigFactoryOption {
	return func(options *proxyEngineConfigFactoryOptions) {
		options.subscriptionClientFactory = factory
	}
}

// ProxyUpstreamConfig holds configuration to configure a single data source to a single upstream.
type ProxyUpstreamConfig struct {
	URL              string
	Method           string
	StaticHeaders    http.Header
	SubscriptionType SubscriptionType
	SSEMethodPost    bool
}

// ProxyEngineConfigFactory is used to create a v2 engine config with a single upstream and a single data source for this upstream.
type ProxyEngineConfigFactory struct {
	httpClient                *http.Client
	streamingClient           *http.Client
	schema                    *Schema
	proxyUpstreamConfig       ProxyUpstreamConfig
	batchFactory              resolve.DataSourceBatchFactory
	subscriptionClientFactory graphqlDataSource.GraphQLSubscriptionClientFactory
	customDirectives          map[string]customdirective.CustomDirective
}

func NewProxyEngineConfigFactory(schema *Schema, proxyUpstreamConfig ProxyUpstreamConfig, batchFactory resolve.DataSourceBatchFactory, opts ...ProxyEngineConfigFactoryOption) *ProxyEngineConfigFactory {
	options := proxyEngineConfigFactoryOptions{
		httpClient: &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1024,
				TLSHandshakeTimeout: 0 * time.Second,
			},
		},
		streamingClient: &http.Client{
			Timeout: 0,
		},
		subscriptionClientFactory: &graphqlDataSource.DefaultSubscriptionClientFactory{},
	}

	for _, optFunc := range opts {
		optFunc(&options)
	}

	return &ProxyEngineConfigFactory{
		httpClient:                options.httpClient,
		streamingClient:           options.streamingClient,
		schema:                    schema,
		proxyUpstreamConfig:       proxyUpstreamConfig,
		batchFactory:              batchFactory,
		subscriptionClientFactory: options.subscriptionClientFactory,
		customDirectives:          options.customDirectives,
	}
}

func (p *ProxyEngineConfigFactory) EngineV2Configuration() (EngineV2Configuration, error) {
	dataSourceConfig := graphqlDataSource.Configuration{
		Fetch: graphqlDataSource.FetchConfiguration{
			URL:    p.proxyUpstreamConfig.URL,
			Method: p.proxyUpstreamConfig.Method,
			Header: p.proxyUpstreamConfig.StaticHeaders,
		},
		Subscription: graphqlDataSource.SubscriptionConfiguration{
			URL:           p.proxyUpstreamConfig.URL,
			UseSSE:        p.proxyUpstreamConfig.SubscriptionType == SubscriptionTypeSSE,
			SSEMethodPost: p.proxyUpstreamConfig.SSEMethodPost,
		},
	}

	conf := NewEngineV2Configuration(p.schema)

	conf.SetCustomDirectives(p.customDirectives)

	rawDoc, report := astparser.ParseGraphqlDocumentBytes(p.schema.rawInput)
	if report.HasErrors() {
		return EngineV2Configuration{}, report
	}

	dataSource, err := newGraphQLDataSourceV2Generator(&rawDoc).Generate(
		dataSourceConfig,
		p.batchFactory,
		p.httpClient,
		WithDataSourceV2GeneratorSubscriptionConfiguration(p.streamingClient, p.proxyUpstreamConfig.SubscriptionType),
		WithDataSourceV2GeneratorSubscriptionClientFactory(p.subscriptionClientFactory),
		WithDataSourceV2GeneratorCustomDirectives(p.customDirectives),
	)
	if err != nil {
		return EngineV2Configuration{}, err
	}

	conf.AddDataSource(dataSource)
	fieldConfigs := newGraphQLFieldConfigsV2Generator(p.schema).Generate()
	conf.SetFieldConfigurations(fieldConfigs)

	return conf, nil
}
