package subscription

import (
	"context"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
)

// Executor is an abstraction for executing a GraphQL engine
type Executor interface {
	Execute(writer resolve.FlushWriter) error
	OperationType() ast.OperationType
	SetContext(context context.Context)
	Reset()
}

// ExecutorPool is an abstraction for creating executors
type ExecutorPool interface {
	Get(payload []byte) (Executor, error)
	Put(executor Executor) error
}
