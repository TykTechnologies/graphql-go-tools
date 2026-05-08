package graphql

import (
	"context"
	"errors"
	"sync"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

var (
	ErrRequiredStagesMissing = errors.New("required stages for custom execution engine v2 are missing")
)

type CustomExecutionEngineV2NormalizerStage interface {
	Normalize(operation *Request) error
}

type CustomExecutionEngineV2ValidatorStage interface {
	ValidateForSchema(operation *Request) error
}

type CustomExecutionEngineV2InputValidationStage interface {
	InputValidation(operation *Request) error
}

type CustomExecutionEngineV2ResolverStage interface {
	Setup(ctx context.Context, postProcessor *postprocess.Processor, resolveContext *resolve.Context, operation *Request, options ...ExecutionOptionsV2)
	Plan(postProcessor *postprocess.Processor, operation *Request, report *operationreport.Report) (plan.Plan, error)
	Resolve(resolveContext *resolve.Context, planResult plan.Plan, writer resolve.SubscriptionResponseWriter) error
	Teardown()
}

type CustomExecutionEngineV2 interface {
	CustomExecutionEngineV2NormalizerStage
	CustomExecutionEngineV2ValidatorStage
	CustomExecutionEngineV2ResolverStage
	CustomExecutionEngineV2InputValidationStage
}

type ExecutionEngineV2Executor interface {
	Execute(ctx context.Context, operation *Request, writer resolve.SubscriptionResponseWriter, options ...ExecutionOptionsV2) error
}

type CustomExecutionEngineV2Stages struct {
	RequiredStages CustomExecutionEngineV2RequiredStages
	OptionalStages *CustomExecutionEngineV2OptionalStages
}

func (c *CustomExecutionEngineV2Stages) AllRequiredStagesProvided() bool {
	return c.RequiredStages.ResolverStage != nil
}

type CustomExecutionEngineV2RequiredStages struct {
	ResolverStage CustomExecutionEngineV2ResolverStage
}

type CustomExecutionEngineV2OptionalStages struct {
	NormalizerStage      CustomExecutionEngineV2NormalizerStage
	ValidatorStage       CustomExecutionEngineV2ValidatorStage
	InputValidationStage CustomExecutionEngineV2InputValidationStage
}

type CustomExecutionEngineV2Executor struct {
	ExecutionStages              CustomExecutionEngineV2Stages
	internalExecutionContextPool sync.Pool
}

func NewCustomExecutionEngineV2Executor(executionEngineV2 CustomExecutionEngineV2) (*CustomExecutionEngineV2Executor, error) {
	executionStages := CustomExecutionEngineV2Stages{
		RequiredStages: CustomExecutionEngineV2RequiredStages{
			ResolverStage: executionEngineV2,
		},
		OptionalStages: &CustomExecutionEngineV2OptionalStages{
			NormalizerStage: executionEngineV2,
			ValidatorStage:  executionEngineV2,
		},
	}

	return NewCustomExecutionEngineV2ExecutorByStages(executionStages)
}

func NewCustomExecutionEngineV2ExecutorByStages(executionStages CustomExecutionEngineV2Stages) (*CustomExecutionEngineV2Executor, error) {
	return &CustomExecutionEngineV2Executor{
		ExecutionStages: executionStages,
		internalExecutionContextPool: sync.Pool{
			New: func() interface{} {
				return newInternalExecutionContext()
			},
		},
	}, nil
}

func (c *CustomExecutionEngineV2Executor) getExecutionCtx() *internalExecutionContext {
	return c.internalExecutionContextPool.Get().(*internalExecutionContext)
}

func (c *CustomExecutionEngineV2Executor) putExecutionCtx(ctx *internalExecutionContext) {
	ctx.reset()
	c.internalExecutionContextPool.Put(ctx)
}

// putExecutionCtxAfterAsyncSubscription releases the wrapper back to the pool
// without calling Free() on the in-flight resolve.Context that has been handed
// off to the asynchronous subscription resolver loop. The resolver retains a
// reference to that context for the lifetime of the subscription; freeing it
// here would race with the resolver goroutine and corrupt live subscription
// state (see TestCustomExecutionEngineV2Executor_AsyncSubscription_NoUseAfterFree).
//
// To keep the pooled wrapper safe for reuse we swap its resolveContext pointer
// for a fresh one so that the next caller does not inherit stale per-request
// fields (Variables, Stats, RenameTypeNames, ...). The original context is now
// owned exclusively by the resolver and will be GC'd when the subscription
// terminates.
func (c *CustomExecutionEngineV2Executor) putExecutionCtxAfterAsyncSubscription(ctx *internalExecutionContext) {
	ctx.resolveContext = resolve.NewContext(context.Background())
	c.internalExecutionContextPool.Put(ctx)
}

func (c *CustomExecutionEngineV2Executor) Execute(ctx context.Context, operation *Request, writer resolve.SubscriptionResponseWriter, options ...ExecutionOptionsV2) error {
	if !c.ExecutionStages.AllRequiredStagesProvided() {
		return ErrRequiredStagesMissing
	}

	var err error
	if c.ExecutionStages.OptionalStages != nil && c.ExecutionStages.OptionalStages.NormalizerStage != nil {
		err = c.ExecutionStages.OptionalStages.NormalizerStage.Normalize(operation)
		if err != nil {
			return err
		}
	}

	if c.ExecutionStages.OptionalStages != nil && c.ExecutionStages.OptionalStages.ValidatorStage != nil {
		err = c.ExecutionStages.OptionalStages.ValidatorStage.ValidateForSchema(operation)
		if err != nil {
			return err
		}
	}

	if c.ExecutionStages.OptionalStages != nil && c.ExecutionStages.OptionalStages.InputValidationStage != nil {
		if err := c.ExecutionStages.OptionalStages.InputValidationStage.InputValidation(operation); err != nil {
			return err
		}
	}

	execContext := c.getExecutionCtx()
	// asyncSubscription is set when planResult is a *plan.SubscriptionResponsePlan
	// because the only resolver path for that plan kind (see ExecutionEngineV2.Resolve)
	// is AsyncResolveGraphQLSubscription, which hands the resolve.Context off to a
	// background goroutine. In that case the deferred release MUST NOT Free() the
	// context — that would race with the resolver loop's xcontext.Detach call and
	// crash subscriptions. See putExecutionCtxAfterAsyncSubscription for details.
	var asyncSubscription bool
	defer func() {
		if asyncSubscription {
			c.putExecutionCtxAfterAsyncSubscription(execContext)
			return
		}
		c.putExecutionCtx(execContext)
	}()
	execContext.prepare(ctx, operation.Variables, operation.request)
	c.ExecutionStages.RequiredStages.ResolverStage.Setup(ctx, execContext.postProcessor, execContext.resolveContext, operation, options...)

	var report operationreport.Report
	planResult, err := c.ExecutionStages.RequiredStages.ResolverStage.Plan(execContext.postProcessor, operation, &report)
	if err != nil {
		return err
	} else if report.HasErrors() {
		return report
	}

	if _, ok := planResult.(*plan.SubscriptionResponsePlan); ok {
		asyncSubscription = true
	}

	err = c.ExecutionStages.RequiredStages.ResolverStage.Resolve(execContext.resolveContext, planResult, writer)
	if err != nil {
		return err
	}

	c.ExecutionStages.RequiredStages.ResolverStage.Teardown()
	return nil
}

// Interface Guards
var (
	_ ExecutionEngineV2Executor = (*CustomExecutionEngineV2Executor)(nil)
)
