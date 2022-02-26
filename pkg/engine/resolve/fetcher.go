package resolve

import (
	"hash"
	"sync"

	"github.com/cespare/xxhash/v2"

	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

type inflightFetch struct {
	waitLoad chan struct{}
	waitFree sync.WaitGroup
	err      error
	bufPair  BufPair
}

type Fetcher struct {
	EnableSingleFlightLoader bool
	hash64Pool               sync.Pool
	inflightFetchPool        sync.Pool
	bufPairPool              sync.Pool
	inflightFetchMu          *sync.Mutex
	inflightFetches          map[uint64]*inflightFetch
}

func NewFetcher(enableSingleFlightLoader bool) *Fetcher {
	return &Fetcher{
		EnableSingleFlightLoader: enableSingleFlightLoader,
		hash64Pool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
		inflightFetchPool: sync.Pool{
			New: func() interface{} {
				return &inflightFetch{
					bufPair: BufPair{
						Data:   fastbuffer.New(),
						Errors: fastbuffer.New(),
					},
				}
			},
		},
		bufPairPool: sync.Pool{
			New: func() interface{} {
				return NewBufPair()
			},
		},
		inflightFetchMu: &sync.Mutex{},
		inflightFetches: map[uint64]*inflightFetch{},
	}
}

func (f *Fetcher) Fetch(ctx *Context, fetch *SingleFetch, preparedInput *fastbuffer.FastBuffer, responseBuf *BufPair) (err error) {
	if ctx.beforeFetchHook != nil {
		ctx.beforeFetchHook.OnBeforeFetch(f.hookCtx(ctx), preparedInput.Bytes())
	}

	if !f.EnableSingleFlightLoader || fetch.DisallowSingleFlight {
		return f.handleFetch(ctx, fetch, preparedInput, responseBuf)
	}

	return f.handleSingleFlight(ctx, fetch, preparedInput, responseBuf)
}

func (f *Fetcher) handleAfterFetchHook(ctx *Context, buf *BufPair, singleFlight bool) {
	if ctx.afterFetchHook != nil {
		if buf.HasData() {
			ctx.afterFetchHook.OnData(f.hookCtx(ctx), buf.Data.Bytes(), singleFlight)
		}
		if buf.HasErrors() {
			ctx.afterFetchHook.OnError(f.hookCtx(ctx), buf.Errors.Bytes(), singleFlight)
		}
	}
}

func (f *Fetcher) handleFetch(ctx *Context, fetch *SingleFetch, preparedInput *fastbuffer.FastBuffer, responseBuf *BufPair) (err error) {
	dataBuf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(dataBuf)

	err = fetch.DataSource.Load(ctx.Context, preparedInput.Bytes(), dataBuf)
	extractResponse(dataBuf.Bytes(), responseBuf, fetch.ProcessResponseConfig)

	f.handleAfterFetchHook(ctx, responseBuf, false)

	return err
}

func (f *Fetcher) writeSingleFlightResult(inflightBuf, responseBuf *BufPair) {
	if inflightBuf.HasData() {
		responseBuf.Data.WriteBytes(inflightBuf.Data.Bytes())
	}
	if inflightBuf.HasErrors() {
		responseBuf.Errors.WriteBytes(inflightBuf.Errors.Bytes())
	}
}

func (f *Fetcher) handleSingleFlight(ctx *Context, fetch *SingleFetch, preparedInput *fastbuffer.FastBuffer, responseBuf *BufPair) (err error) {
	hash64 := f.getHash64()
	_, _ = hash64.Write(preparedInput.Bytes())
	fetchID := hash64.Sum64()
	f.putHash64(hash64)

	f.inflightFetchMu.Lock()
	inflight, ok := f.inflightFetches[fetchID]
	if ok {
		inflight.waitFree.Add(1)
		defer inflight.waitFree.Done()
		f.inflightFetchMu.Unlock()

		select {
		case <-ctx.Context.Done():
			return ctx.Context.Err()
		case <-inflight.waitLoad:
			break
		}

		f.handleAfterFetchHook(ctx, &inflight.bufPair, true)
		f.writeSingleFlightResult(&inflight.bufPair, responseBuf)
		return inflight.err
	}

	inflight = f.getInflightFetch()
	inflight.waitLoad = make(chan struct{})
	f.inflightFetches[fetchID] = inflight
	f.inflightFetchMu.Unlock()

	err = f.handleFetch(ctx, fetch, preparedInput, &inflight.bufPair)
	inflight.err = err
	f.writeSingleFlightResult(&inflight.bufPair, responseBuf)
	close(inflight.waitLoad)

	f.inflightFetchMu.Lock()
	delete(f.inflightFetches, fetchID)
	f.inflightFetchMu.Unlock()

	go func() {
		inflight.waitFree.Wait()
		f.freeInflightFetch(inflight)
	}()

	return
}

func (f *Fetcher) FetchBatch(ctx *Context, fetch *BatchFetch, preparedInputs []*fastbuffer.FastBuffer, bufs []*BufPair) (err error) {
	inputs := make([][]byte, len(preparedInputs))
	for i := range preparedInputs {
		inputs[i] = preparedInputs[i].Bytes()
	}

	batch, err := fetch.BatchFactory.CreateBatch(inputs)
	if err != nil {
		return err
	}

	buf := f.getBufPair()
	defer f.freeBufPair(buf)

	if err = f.Fetch(ctx, fetch.Fetch, batch.Input(), buf); err != nil {
		return err
	}

	if err = batch.Demultiplex(buf, bufs); err != nil {
		return err
	}

	return
}

func (f *Fetcher) getBufPair() *BufPair {
	return f.bufPairPool.Get().(*BufPair)
}

func (f *Fetcher) freeBufPair(buf *BufPair) {
	buf.Reset()
	f.bufPairPool.Put(buf)
}

func (f *Fetcher) getInflightFetch() *inflightFetch {
	return f.inflightFetchPool.Get().(*inflightFetch)
}

func (f *Fetcher) freeInflightFetch(inflightFetch *inflightFetch) {
	inflightFetch.bufPair.Data.Reset()
	inflightFetch.bufPair.Errors.Reset()
	inflightFetch.err = nil
	f.inflightFetchPool.Put(inflightFetch)
}

func (f *Fetcher) hookCtx(ctx *Context) HookContext {
	return HookContext{
		CurrentPath: ctx.path(),
	}
}

func (f *Fetcher) getHash64() hash.Hash64 {
	return f.hash64Pool.Get().(hash.Hash64)
}

func (f *Fetcher) putHash64(h hash.Hash64) {
	h.Reset()
	f.hash64Pool.Put(h)
}
