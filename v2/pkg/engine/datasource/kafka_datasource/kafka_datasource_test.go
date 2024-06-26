package kafka_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/IBM/sarama"
	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

const (
	testMockKafkaVersion = "V2_8_0_0"
	testDefinition       = `
schema {
    subscription: Subscription
}

type Subscription {
    remainingJedis: Int!
}
`
)

type runTestOnTestDefinitionOptions func(planConfig *plan.Configuration, extraChecks []datasourcetesting.CheckFunc)

func runTestOnTestDefinition(operation, operationName string, expectedPlan plan.Plan, options ...runTestOnTestDefinitionOptions) func(t *testing.T) {
	extraChecks := make([]datasourcetesting.CheckFunc, 0)
	config := plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Subscription",
						FieldNames: []string{"remainingJedis"},
					},
				},
				Custom: ConfigJSON(Configuration{
					Subscription: SubscriptionConfiguration{
						BrokerAddresses: []string{"localhost:9092"},
						Topics:          []string{"test.topic"},
						GroupID:         "test.consumer.group",
						ClientID:        "test.client.id",
						KafkaVersion:    testMockKafkaVersion,
						BalanceStrategy: DefaultBalanceStrategy,
						IsolationLevel:  DefaultIsolationLevel,
						SASL: SASL{
							Enable:   true,
							User:     testSASLUser,
							Password: testSASLPassword,
						},
					},
				}),
				Factory: &Factory{},
			},
		},
	}

	for _, opt := range options {
		opt(&config, extraChecks)
	}

	return datasourcetesting.RunTest(testDefinition, operation, operationName, expectedPlan, config, datasourcetesting.WithCheckFuncs(extraChecks...))
}

func testWithFactory(factory *Factory) runTestOnTestDefinitionOptions {
	return func(planConfig *plan.Configuration, extraChecks []datasourcetesting.CheckFunc) {
		for _, ds := range planConfig.DataSources {
			ds.Factory = factory
		}
	}
}

func TestKafkaDataSource(t *testing.T) {
	factory := &Factory{}
	ctx := resolve.NewContext(context.Background())

	t.Run("subscription", runTestOnTestDefinition(`
		subscription RemainingJedis {
			remainingJedis
		}
	`, "RemainingJedis", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(fmt.Sprintf(`{"broker_addresses":["localhost:9092"],"topics":["test.topic"],"group_id":"test.consumer.group","client_id":"test.client.id","kafka_version":"%s","start_consuming_latest":false,"balance_strategy":"%s","isolation_level":"%s","sasl":{"enable":true,"user":"%s","password":"%s"}}`,
					testMockKafkaVersion,
					DefaultBalanceStrategy,
					DefaultIsolationLevel,
					testSASLUser,
					testSASLPassword,
				)),
				Source: &SubscriptionSource{
					client: NewKafkaConsumerGroupBridge(ctx.Context(), logger()),
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("remainingJedis"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Integer{
								Path:     []string{"remainingJedis"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}, testWithFactory(factory)))

	t.Run("subscription with variables", datasourcetesting.RunTest(`
		type Subscription {
			foo(bar: String): Int!
 		}
`, `
		subscription SubscriptionWithVariables($bar: String) {
			foo(bar: $bar)
		}
	`, "SubscriptionWithVariables", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(fmt.Sprintf(`{"broker_addresses":["localhost:9092"],"topics":["test.topic.$$0$$"],"group_id":"test.consumer.group","client_id":"test.client.id","kafka_version":"%s","start_consuming_latest":false,"balance_strategy":"%s","isolation_level":"%s","sasl":{"enable":true,"user":"%s","password":"%s"}}`,
					testMockKafkaVersion,
					DefaultBalanceStrategy,
					DefaultIsolationLevel,
					testSASLUser,
					testSASLPassword,
				)),
				Variables: resolve.NewVariables(
					&resolve.ContextVariable{
						Path:     []string{"bar"},
						Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["string","null"]}`),
					},
				),
				Source: &SubscriptionSource{
					client: NewKafkaConsumerGroupBridge(ctx.Context(), logger()),
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("foo"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Integer{
								Path:     []string{"foo"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Subscription",
						FieldNames: []string{"foo"},
					},
				},
				Custom: ConfigJSON(Configuration{
					Subscription: SubscriptionConfiguration{
						BrokerAddresses: []string{"localhost:9092"},
						Topics:          []string{"test.topic.{{.arguments.bar}}"},
						GroupID:         "test.consumer.group",
						ClientID:        "test.client.id",
						KafkaVersion:    testMockKafkaVersion,
						BalanceStrategy: DefaultBalanceStrategy,
						IsolationLevel:  DefaultIsolationLevel,
						SASL: SASL{
							Enable:   true,
							User:     testSASLUser,
							Password: testSASLPassword,
						},
					},
				}),
				Factory: factory,
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Subscription",
				FieldName: "foo",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "bar",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
	}))
}

var errSubscriptionClientFail = errors.New("subscription client fail error")

type FailingSubscriptionClient struct{}

func (f FailingSubscriptionClient) UniqueRequestID(ctx *resolve.Context, options GraphQLSubscriptionOptions, hash *xxhash.Digest) (err error) {
	_, err = hash.Write([]byte("test"))
	return err
}

func (f FailingSubscriptionClient) Subscribe(_ *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.CloseableSubscriptionUpdater) error {
	return errSubscriptionClientFail
}

func TestKafkaDataSource_Subscription_Start(t *testing.T) {
	newSubscriptionSource := func(ctx context.Context) SubscriptionSource {
		subscriptionSource := SubscriptionSource{client: NewKafkaConsumerGroupBridge(ctx, abstractlogger.NoopLogger)}
		return subscriptionSource
	}

	t.Run("should return error when input is invalid", func(t *testing.T) {
		ctx := resolve.NewContext(context.Background())

		source := SubscriptionSource{client: FailingSubscriptionClient{}}
		err := source.Start(ctx, []byte(`{"broker_addresses":"",topic":"","group_id":""}`), nil)
		assert.Error(t, err)
	})

	t.Run("should send and receive a message, then cancel subscription", func(t *testing.T) {
		resolveCtx := resolve.NewContext(context.Background())
		kafkaCtx, kafkaCtxCancel := context.WithCancel(context.Background())

		topic := "graphql-go-tools.test.topic"
		groupID := "graphql-go-tools.test.groupid"
		source := newSubscriptionSource(kafkaCtx)

		fr := sarama.NewMockFetchResponse(t, 1)
		mockBroker := newMockKafkaBroker(t, topic, groupID, fr)
		defer mockBroker.Close()

		options := GraphQLSubscriptionOptions{
			BrokerAddresses: []string{mockBroker.Addr()},
			Topics:          []string{topic},
			GroupID:         groupID,
			ClientID:        "graphql-go-tools.test.groupid",
			KafkaVersion:    testMockKafkaVersion,
		}
		optionsBytes, err := json.Marshal(options)
		require.NoError(t, err)

		next := make(chan []byte)
		subscriptionUpdater := newTestSubscriptionUpdater(next)
		err = source.Start(resolveCtx, optionsBytes, subscriptionUpdater)
		require.NoError(t, err)

		testMessageKey := sarama.StringEncoder("test.message.key")
		testMessageValue := sarama.StringEncoder(`{"stock":[{"name":"Trilby","price":293,"inStock":2}]}`)

		// Add a message to the topic. The consumer group will fetch that message and trigger ConsumeClaim method.
		fr.SetMessageWithKey(topic, defaultPartition, 0, testMessageKey, testMessageValue)

		nextBytes := <-next
		assert.Equal(t, `{"data":{"stock":[{"name":"Trilby","price":293,"inStock":2}]}}`, string(nextBytes))

		kafkaCtxCancel()

		_, ok := <-next
		assert.False(t, ok)
	})
}

func TestKafkaConsumerGroupBridge_Subscribe(t *testing.T) {
	var (
		testMessageKey   = sarama.StringEncoder("test.message.key")
		testMessageValue = sarama.StringEncoder(`{"stock":[{"name":"Trilby","price":293,"inStock":2}]}`)
		topic            = "test.topic"
		consumerGroup    = "consumer.group"
	)

	fr := sarama.NewMockFetchResponse(t, 1)
	mockBroker := newMockKafkaBroker(t, topic, consumerGroup, fr)
	defer mockBroker.Close()

	// Add a message to the topic. The consumer group will fetch that message and trigger ConsumeClaim method.
	fr.SetMessageWithKey(topic, defaultPartition, 0, testMessageKey, testMessageValue)

	ctx := resolve.NewContext(context.Background())
	cg := NewKafkaConsumerGroupBridge(ctx.Context(), logger()) // use abstractlogger.NoopLogger if there is no available logger.
	options := GraphQLSubscriptionOptions{
		BrokerAddresses: []string{mockBroker.Addr()},
		Topics:          []string{topic},
		GroupID:         consumerGroup,
		ClientID:        "graphql-go-tools-test",
		KafkaVersion:    testMockKafkaVersion,
	}

	next := make(chan []byte)
	subscriptionUpdater := newTestSubscriptionUpdater(next)
	err := cg.Subscribe(ctx, options, subscriptionUpdater)
	require.NoError(t, err)

	msg := <-next
	expectedMsg, err := testMessageValue.Encode()
	require.NoError(t, err)

	value, _, _, err := jsonparser.Get(msg, "data")
	require.NoError(t, err)
	require.Equal(t, expectedMsg, value)
}

type testSubscriptionUpdater struct {
	next chan []byte
}

func newTestSubscriptionUpdater(next chan []byte) *testSubscriptionUpdater {
	return &testSubscriptionUpdater{
		next: next,
	}
}

func (t *testSubscriptionUpdater) Update(data []byte) {
	t.next <- data
}

func (t *testSubscriptionUpdater) Done() {
	close(t.next)
}

func (t *testSubscriptionUpdater) Close() {
	close(t.next)
}
