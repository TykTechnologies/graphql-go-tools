package subscription

import (
	"context"
	"sync"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
)

type Engine interface {
	StartOperation(ctx context.Context, id string, payload []byte, eventHandler EventHandler) error
	StopSubscription(id string, eventHandler EventHandler) error
}

type ExecutorEngine struct {
	logger abstractlogger.Logger
	// subCancellations is map containing the cancellation functions to every active subscription.
	subCancellations subscriptionCancellations
	// executorPool is responsible to create and hold executors.
	executorPool ExecutorPool
	// bufferPool will hold buffers.
	bufferPool *sync.Pool
	// subscriptionUpdateInterval is the actual interval on which the server sends subscription updates to the client.
	subscriptionUpdateInterval time.Duration
}

func (e *ExecutorEngine) StartOperation(ctx context.Context, id string, payload []byte, eventHandler EventHandler) error {
	executor, err := e.executorPool.Get(payload)
	if err != nil {
		return err
	}

	if err = e.handleOnBeforeStart(executor); err != nil {
		return err
	}

	if executor.OperationType() == ast.OperationTypeSubscription {
		ctx, subsErr := e.subCancellations.AddWithParent(id, ctx)
		if subsErr != nil {
			eventHandler.Emit(EventTypeError, id, nil, subsErr)
			return subsErr
		}
		go e.startSubscription(ctx, id, executor, eventHandler)
		return nil
	}

	go e.handleNonSubscriptionOperation(ctx, id, executor, eventHandler)
	return nil
}

func (e *ExecutorEngine) StopSubscription(id string, eventHandler EventHandler) error {
	e.subCancellations.Cancel(id)
	eventHandler.Emit(EventTypeCompleted, id, nil, nil)
	return nil
}

func (e *ExecutorEngine) handleOnBeforeStart(executor Executor) error {
	switch e := executor.(type) {
	case *ExecutorV2:
		if hook := e.engine.GetWebsocketBeforeStartHook(); hook != nil {
			return hook.OnBeforeStart(e.reqCtx, e.operation)
		}
	case *ExecutorV1:
		// do nothing
	}

	return nil
}

func (e *ExecutorEngine) startSubscription(ctx context.Context, id string, executor Executor, eventHandler EventHandler) {
	defer func() {
		err := e.executorPool.Put(executor)
		if err != nil {
			e.logger.Error("subscription.Handle.startSubscription()",
				abstractlogger.Error(err),
			)
		}
	}()

	executor.SetContext(ctx)
	buf := e.bufferPool.Get().(*graphql.EngineResultWriter)
	buf.Reset()

	defer e.bufferPool.Put(buf)

	e.executeSubscription(buf, id, executor, eventHandler)

	for {
		buf.Reset()
		select {
		case <-ctx.Done():
			return
		case <-time.After(e.subscriptionUpdateInterval):
			e.executeSubscription(buf, id, executor, eventHandler)
		}
	}

}

func (e *ExecutorEngine) executeSubscription(buf *graphql.EngineResultWriter, id string, executor Executor, eventHandler EventHandler) {
	buf.SetFlushCallback(func(data []byte) {
		e.logger.Debug("subscription.Handle.executeSubscription()",
			abstractlogger.ByteString("execution_result", data),
		)
		eventHandler.Emit(EventTypeData, id, data, nil)
	})
	defer buf.SetFlushCallback(nil)

	err := executor.Execute(buf)
	if err != nil {
		e.logger.Error("subscription.Handle.executeSubscription()",
			abstractlogger.Error(err),
		)

		eventHandler.Emit(EventTypeError, id, nil, err)
		return
	}

	if buf.Len() > 0 {
		data := buf.Bytes()
		e.logger.Debug("subscription.Handle.executeSubscription()",
			abstractlogger.ByteString("execution_result", data),
		)
		eventHandler.Emit(EventTypeData, id, data, nil)
	}
}

func (e *ExecutorEngine) handleNonSubscriptionOperation(ctx context.Context, id string, executor Executor, eventHandler EventHandler) {
	defer func() {
		err := e.executorPool.Put(executor)
		if err != nil {
			e.logger.Error("subscription.Handle.handleNonSubscriptionOperation()",
				abstractlogger.Error(err),
			)
		}
	}()

	executor.SetContext(ctx)
	buf := e.bufferPool.Get().(*graphql.EngineResultWriter)
	buf.Reset()

	defer e.bufferPool.Put(buf)

	err := executor.Execute(buf)
	if err != nil {
		e.logger.Error("subscription.Handle.handleNonSubscriptionOperation()",
			abstractlogger.Error(err),
		)

		eventHandler.Emit(EventTypeError, id, nil, err)
		return
	}

	e.logger.Debug("subscription.Handle.handleNonSubscriptionOperation()",
		abstractlogger.ByteString("execution_result", buf.Bytes()),
	)

	eventHandler.Emit(EventTypeData, id, buf.Bytes(), err)
	eventHandler.Emit(EventTypeCompleted, id, nil, nil)
}
