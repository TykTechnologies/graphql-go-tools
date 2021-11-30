package graphql_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

func BenchmarkFederationBatchingFastJson(b *testing.B) {

	userService := FakeDataSource(`{"data":{"me": {"id": "1234","username": "Me","__typename": "User"}}}`)

	reviews := strings.Builder{}

	count := 100
	last := count - 1

	reviews.Write(
		[]byte(`
{
    "data": {
      "_entities": [
        {
          "reviews": [`))

	for i := 0; i < count; i++ {
		reviews.Write(
			[]byte(fmt.Sprintf(`
            {
              "body": "Doloremque quasi animi sed eum.",
              "product": {
                "upc": "top-%d"
              }
            }`, i+1)))

		if i != last {
			reviews.Write([]byte(`,`))
		}

	}

	reviews.Write(
		[]byte(`
          ]
        }
      ]
    }
  }
`))

	reviewsResp := reviews.String()
	buf := bytes.Buffer{}
	assert.NoError(b, json.Compact(&buf, []byte(reviewsResp)))
	reviewsResp = buf.String()
	reviewsService := FakeDataSource(reviewsResp)

	products := strings.Builder{}

	products.Write([]byte(`
 {
    "data": {
      "_entities": [`))

	for i := 0; i < count; i++ {
		products.Write([]byte(`
        {
          "name": "Trilby"
        }`))

		if i != last {
			products.Write([]byte(`,`))
		}
	}

	products.Write([]byte(`
      ]
    }
  }
`))

	productsResp := products.String()

	buf = bytes.Buffer{}
	assert.NoError(b, json.Compact(&buf, []byte(productsResp)))

	productsResp = buf.String()
	productsService := FakeDataSource(productsResp)

	reviewBatchFactory := NewBatchFactory()
	productBatchFactory := NewBatchFactory()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := resolve.NewFastJson(ctx, resolve.NewFetcher(true), true)

	preparedPlan := &resolve.GraphQLResponse{
		Data: &resolve.Object{
			Fetch: &resolve.SingleFetch{
				BufferId: 0,
				InputTemplate: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
							SegmentType: resolve.StaticSegmentType,
						},
					},
				},
				DataSource: userService,
				ProcessResponseConfig: resolve.ProcessResponseConfig{
					ExtractGraphqlResponse: true,
				},
			},
			Fields: []*resolve.Field{
				{
					HasBuffer: true,
					BufferID:  0,
					Name:      []byte("me"),
					Value: &resolve.Object{
						Fetch: &resolve.BatchFetch{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								InputTemplate: resolve.InputTemplate{
									Segments: []resolve.TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"`),
											SegmentType: resolve.StaticSegmentType,
										},
										{
											SegmentType:                  resolve.VariableSegmentType,
											VariableSource:               resolve.VariableSourceObject,
											VariableSourcePath:           []string{"id"},
											VariableValueType:            jsonparser.Number,
											RenderVariableAsGraphQLValue: true,
										},
										{
											Data:        []byte(`","__typename":"User"}]}}}`),
											SegmentType: resolve.StaticSegmentType,
										},
									},
								},
								DataSource: reviewsService,
								ProcessResponseConfig: resolve.ProcessResponseConfig{
									ExtractGraphqlResponse:    true,
									ExtractFederationEntities: true,
								},
							},
							BatchFactory: reviewBatchFactory,
						},
						Path:     []string{"me"},
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("id"),
								Value: &resolve.String{
									Path: []string{"id"},
								},
							},
							{
								Name: []byte("username"),
								Value: &resolve.String{
									Path: []string{"username"},
								},
							},
							{

								HasBuffer: true,
								BufferID:  1,
								Name:      []byte("reviews"),
								Value: &resolve.Array{
									Path:     []string{"reviews"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("body"),
												Value: &resolve.String{
													Path: []string{"body"},
												},
											},
											{
												Name: []byte("product"),
												Value: &resolve.Object{
													Path: []string{"product"},
													Fetch: &resolve.BatchFetch{
														Fetch: &resolve.SingleFetch{
															BufferId:   2,
															DataSource: productsService,
															InputTemplate: resolve.InputTemplate{
																Segments: []resolve.TemplateSegment{
																	{
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																	{
																		SegmentType:                  resolve.VariableSegmentType,
																		VariableSource:               resolve.VariableSourceObject,
																		VariableSourcePath:           []string{"upc"},
																		VariableValueType:            jsonparser.String,
																		RenderVariableAsGraphQLValue: true,
																	},
																	{
																		Data:        []byte(`,"__typename":"Product"}]}}}`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																},
															},
															ProcessResponseConfig: resolve.ProcessResponseConfig{
																ExtractGraphqlResponse:    true,
																ExtractFederationEntities: true,
															},
														},
														BatchFactory: productBatchFactory,
													},
													Fields: []*resolve.Field{
														{
															Name: []byte("upc"),
															Value: &resolve.String{
																Path: []string{"upc"},
															},
														},
														{
															HasBuffer: true,
															BufferID:  2,
															Name:      []byte("name"),
															Value: &resolve.String{
																Path: []string{"name"},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	var err error
	// expected := []byte(`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`)

	expected := []byte(`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-2","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-3","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-4","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-5","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-6","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-7","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-8","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-9","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-10","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-11","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-12","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-13","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-14","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-15","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-16","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-17","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-18","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-19","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-20","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-21","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-22","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-23","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-24","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-25","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-26","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-27","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-28","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-29","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-30","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-31","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-32","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-33","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-34","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-35","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-36","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-37","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-38","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-39","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-40","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-41","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-42","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-43","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-44","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-45","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-46","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-47","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-48","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-49","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-50","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-51","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-52","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-53","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-54","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-55","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-56","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-57","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-58","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-59","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-60","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-61","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-62","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-63","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-64","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-65","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-66","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-67","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-68","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-69","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-70","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-71","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-72","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-73","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-74","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-75","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-76","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-77","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-78","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-79","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-80","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-81","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-82","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-83","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-84","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-85","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-86","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-87","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-88","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-89","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-90","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-91","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-92","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-93","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-94","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-95","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-96","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-97","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-98","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-99","name":"Trilby"}},{"body":"Doloremque quasi animi sed eum.","product":{"upc":"top-100","name":"Trilby"}}]}}}`)

	pool := sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	ctxPool := sync.Pool{
		New: func() interface{} {
			return resolve.NewContext(context.Background())
		},
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// _ = resolver.ResolveGraphQLResponse(ctx, plan, nil, ioutil.Discard)
			ctx := ctxPool.Get().(*resolve.Context)
			buf := pool.Get().(*bytes.Buffer)
			err = resolver.ResolveGraphQLResponse(ctx, preparedPlan, nil, buf)
			if err != nil {
				b.Fatal(err)
			}
			if !bytes.Equal(expected, buf.Bytes()) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), buf.String())
			}

			buf.Reset()
			pool.Put(buf)

			ctx.Free()
			ctxPool.Put(ctx)
		}
	})
}
