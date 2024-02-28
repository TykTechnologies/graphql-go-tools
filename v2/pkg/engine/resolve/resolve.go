//go:generate mockgen --build_flags=--mod=mod -self_package=github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource,BeforeFetchHook,AfterFetchHook

package resolve

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/buger/jsonparser"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/pool"
)

type Resolver struct {
	ctx                      context.Context
	enableSingleFlightLoader bool
	toolPool                 sync.Pool
}

type tools struct {
	resolvable *Resolvable
	loader     *Loader
}

// New returns a new Resolver, ctx.Done() is used to cancel all active subscriptions & streams
func New(ctx context.Context) *Resolver {
	return &Resolver{
		ctx: ctx,
		toolPool: sync.Pool{
			New: func() interface{} {
				return &tools{
					resolvable: NewResolvable(),
					loader:     &Loader{},
				}
			},
		},
	}
}

func (r *Resolver) getTools() *tools {
	t := r.toolPool.Get().(*tools)
	t.loader.enableSingleFlight = r.enableSingleFlightLoader
	return t
}

func (r *Resolver) putTools(t *tools) {
	t.loader.Free()
	t.resolvable.Reset()
	r.toolPool.Put(t)
}

func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (err error) {
	if response.Info == nil {
		response.Info = &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		}
	}

	t := r.getTools()
	defer r.putTools(t)

	err = t.resolvable.Init(ctx, data, response.Info.OperationType)
	if err != nil {
		return err
	}

	err = t.loader.LoadGraphQLResponseData(ctx, response, t.resolvable)
	if err != nil {
		return err
	}

	return t.resolvable.Resolve(ctx.ctx, response.Data, writer)
}

func (r *Resolver) ResolveGraphQLSubscription(ctx *Context, subscription *GraphQLSubscription, writer FlushWriter) (err error) {

	if subscription.Trigger.Source == nil {
		msg := []byte(`{"errors":[{"message":"no data source found"}]}`)
		return writeAndFlush(writer, msg)
	}

	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)
	if err := subscription.Trigger.InputTemplate.Render(ctx, nil, buf); err != nil {
		return err
	}
	rendered := buf.Bytes()
	subscriptionInput := make([]byte, len(rendered))
	copy(subscriptionInput, rendered)

	if len(ctx.InitialPayload) > 0 {
		subscriptionInput, err = jsonparser.Set(subscriptionInput, ctx.InitialPayload, "initial_payload")
		if err != nil {
			return err
		}
	}

	if ctx.Extensions != nil {
		subscriptionInput, err = jsonparser.Set(subscriptionInput, ctx.Extensions, "body", "extensions")
	}

	c, cancel := context.WithCancel(ctx.Context())
	defer cancel()
	resolverDone := r.ctx.Done()

	next := make(chan []byte)

	cancellableContext := ctx.WithContext(c)

	if err := subscription.Trigger.Source.Start(cancellableContext, subscriptionInput, next); err != nil {
		if errors.Is(err, ErrUnableToResolve) {
			msg := []byte(`{"errors":[{"message":"unable to resolve"}]}`)
			return writeAndFlush(writer, msg)
		}
		return err
	}

	t := r.getTools()
	defer r.putTools(t)

	for {
		select {
		case <-resolverDone:
			return nil
		case data, ok := <-next:
			if !ok {
				return nil
			}
			t.resolvable.Reset()
			if err := t.resolvable.InitSubscription(ctx, data, subscription.Trigger.PostProcessing); err != nil {
				return err
			}
			if err := t.loader.LoadGraphQLResponseData(ctx, subscription.Response, t.resolvable); err != nil {
				return err
			}
			if err := t.resolvable.Resolve(ctx.ctx, subscription.Response.Data, writer); err != nil {
				return err
			}
			writer.Flush()
		}
	}
}
