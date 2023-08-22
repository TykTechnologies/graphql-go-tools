package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
)

func TestTimeOutChecker(t *testing.T) {
	t.Run("should stop timer if context is done before", func(t *testing.T) {
		timeOutActionExecuted := false
		timeOutAction := func() {
			timeOutActionExecuted = true
		}

		timeOutCtx, timeOutCancel := context.WithCancel(context.Background())
		params := TimeOutParams{
			Name:            "",
			Logger:          abstractlogger.Noop{},
			TimeOutContext:  timeOutCtx,
			TimeOutAction:   timeOutAction,
			TimeOutDuration: 5 * time.Millisecond,
		}
		go TimeOutChecker(params)
		time.Sleep(2 * time.Millisecond)
		timeOutCancel()
		time.Sleep(5 * time.Millisecond)
		assert.False(t, timeOutActionExecuted)
	})

	t.Run("should stop process if timer runs out", func(t *testing.T) {
		timeOutActionExecuted := false
		timeOutAction := func() {
			timeOutActionExecuted = true
		}

		timeOutCtx, timeOutCancel := context.WithCancel(context.Background())
		defer timeOutCancel()

		params := TimeOutParams{
			Name:            "",
			Logger:          abstractlogger.Noop{},
			TimeOutContext:  timeOutCtx,
			TimeOutAction:   timeOutAction,
			TimeOutDuration: 10 * time.Millisecond,
		}
  		go TimeOutChecker(params)
  		time.Sleep(20 * time.Millisecond)
  		assert.True(t, timeOutActionExecuted)
	})
}