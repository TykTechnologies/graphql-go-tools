package graphql

import (
	"context"
	"errors"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
)

// ObservableExecutionStages defines the stages of a GraphQL query execution.
type ObservableExecutionStages interface {
	Setup(ctx context.Context, operation *Request, options ...ExecutionOptionsV2)
	Normalize(operation *Request) error
	ValidateForSchema(operation *Request) error
	Plan(operation *Request, report *operationreport.Report) (plan.Plan, error)
	Resolve(plan plan.Plan, writer resolve.FlushWriter) error
	Teardown()
}

// ObservableExecutionEngine defines a GraphQL query executor.
type ObservableExecutionEngine interface {
	Context() context.Context
	Execute(ctx context.Context, operation *Request, writer resolve.FlushWriter, options ...ExecutionOptionsV2) error
}

type ObservableExecutionEngineV2 struct {
	stages ObservableExecutionStages
}

type ObservableExecutionStagesV2 struct {
	executionContext  *internalExecutionContext
	executionEngineV2 *ExecutionEngineV2
}

func (e *ObservableExecutionStagesV2) Setup(ctx context.Context, operation *Request, options ...ExecutionOptionsV2) {
	execContext := e.executionEngineV2.getExecutionCtx()

	execContext.prepare(ctx, operation.Variables, operation.request)

	for i := range options {
		options[i](execContext)
	}
	e.executionContext = execContext
}

func (e *ObservableExecutionStagesV2) Teardown() {
	e.executionEngineV2.putExecutionCtx(e.executionContext)
	e.executionContext = nil
}

func (e *ObservableExecutionStagesV2) Normalize(request *Request) error {
	result, err := request.Normalize(e.executionEngineV2.config.schema)
	if err != nil {
		return err
	}

	if !result.Successful {
		return result.Errors
	}

	return nil
}

func (e *ObservableExecutionStagesV2) ValidateForSchema(request *Request) error {
	result, err := request.ValidateForSchema(e.executionEngineV2.config.schema)
	if err != nil {
		return err
	}
	if !result.Valid {
		return result.Errors
	}

	return nil
}

func (e *ObservableExecutionStagesV2) Plan(operation *Request, report *operationreport.Report) (plan.Plan, error) {
	cachedPlan := e.executionEngineV2.getCachedPlan(e.executionContext, &operation.document, &e.executionEngineV2.config.schema.document, operation.OperationName, report)
	if report.HasErrors() {
		return nil, report
	}
	return cachedPlan, nil
}

func (e *ObservableExecutionStagesV2) Resolve(executionPlan plan.Plan, writer resolve.FlushWriter) error {
	var err error
	switch p := executionPlan.(type) {
	case *plan.SynchronousResponsePlan:
		err = e.executionEngineV2.resolver.ResolveGraphQLResponse(e.executionContext.resolveContext, p.Response, nil, writer)
	case *plan.SubscriptionResponsePlan:
		err = e.executionEngineV2.resolver.ResolveGraphQLSubscription(e.executionContext.resolveContext, p.Response, writer)
	default:
		return errors.New("execution of operation is not possible")
	}

	return err
}

func NewObservableExecutionStagesV2(engineV2 *ExecutionEngineV2) *ObservableExecutionStagesV2 {
	return &ObservableExecutionStagesV2{
		executionEngineV2: engineV2,
	}
}

// NewObservableExecutionEngineV2 takes ObservableExecutionStages interface as argument. So the library user
// can provide any implementation of the execution stages.
func NewObservableExecutionEngineV2(stages ObservableExecutionStages) ObservableExecutionEngine {
	return &ObservableExecutionEngineV2{
		stages: stages,
	}
}

func (e *ObservableExecutionEngineV2) Context() context.Context {
	return context.Background()
}

func (e *ObservableExecutionEngineV2) Execute(ctx context.Context, operation *Request, writer resolve.FlushWriter, options ...ExecutionOptionsV2) error {
	// ObservableExecutionEngineV2 is mainly a wrapper around the existing ExecutionEngineV2 implementation.
	// We start the same methods in the same order but make them observable.
	e.stages.Setup(ctx, operation, options...)
	defer e.stages.Teardown()

	if !operation.IsNormalized() {
		err := e.stages.Normalize(operation)
		if err != nil {
			return err
		}
	}

	err := e.stages.ValidateForSchema(operation)
	if err != nil {
		return err
	}

	var report operationreport.Report
	executionPlan, err := e.stages.Plan(operation, &report)
	if err != nil {
		return err
	}

	return e.stages.Resolve(executionPlan, writer)
}

var (
	_ ObservableExecutionStages = (*ObservableExecutionStagesV2)(nil)
	_ ObservableExecutionEngine = (*ObservableExecutionEngineV2)(nil)
)
