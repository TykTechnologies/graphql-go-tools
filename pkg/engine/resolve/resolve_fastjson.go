package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	errors "golang.org/x/xerrors"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve/fastjson"

	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

var FastJsonParser = fastjson.ParserPool{}

type ResolverFastJson struct {
	ctx               context.Context
	dataLoaderEnabled bool
	resultSetPool     sync.Pool
	byteSlicesPool    sync.Pool
	waitGroupPool     sync.Pool
	bufPairPool       sync.Pool
	bufPairSlicePool  sync.Pool
	errChanPool       sync.Pool
	hash64Pool        sync.Pool
	dataloaderFactory *dataLoaderFactory
	fetcher           *Fetcher
}

// NewFastJson returns a new Resolver, ctx.Done() is used to cancel all active subscriptions & streams
func NewFastJson(ctx context.Context, fetcher *Fetcher, enableDataLoader bool) *ResolverFastJson {
	return &ResolverFastJson{
		ctx: ctx,
		resultSetPool: sync.Pool{
			New: func() interface{} {
				return &resultSet{
					buffers: make(map[int]*BufPair, 8),
				}
			},
		},
		byteSlicesPool: sync.Pool{
			New: func() interface{} {
				slice := make([][]byte, 0, 24)
				return &slice
			},
		},
		waitGroupPool: sync.Pool{
			New: func() interface{} {
				return &sync.WaitGroup{}
			},
		},
		bufPairPool: sync.Pool{
			New: func() interface{} {
				pair := BufPair{
					Data:   fastbuffer.New(),
					Errors: fastbuffer.New(),
				}
				return &pair
			},
		},
		bufPairSlicePool: sync.Pool{
			New: func() interface{} {
				slice := make([]*BufPair, 0, 24)
				return &slice
			},
		},
		errChanPool: sync.Pool{
			New: func() interface{} {
				return make(chan error, 1)
			},
		},
		hash64Pool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
		dataloaderFactory: newDataloaderFactory(fetcher),
		fetcher:           fetcher,
		dataLoaderEnabled: enableDataLoader,
	}
}

func (r *ResolverFastJson) resolveNode(ctx *Context, node Node, data *fastjson.Value, bufPair *BufPair) (err error) {
	switch n := node.(type) {
	case *Object:
		return r.resolveObject(ctx, n, data, bufPair)
	case *Array:
		return r.resolveArray(ctx, n, data, bufPair)
	case *Null:
		// if n.Defer.Enabled {
		// 	r.preparePatch(ctx, n.Defer.PatchIndex, nil, data)
		// }
		r.resolveNull(bufPair.Data)
		return
	case *String:
		return r.resolveString(n, data, bufPair)
	case *Boolean:
		return r.resolveBoolean(n, data, bufPair)
	case *Integer:
		return r.resolveInteger(n, data, bufPair)
	case *Float:
		return r.resolveFloat(n, data, bufPair)
	case *EmptyObject:
		r.resolveEmptyObject(bufPair.Data)
		return
	case *EmptyArray:
		r.resolveEmptyArray(bufPair.Data)
		return
	default:
		return
	}
}

func (r *ResolverFastJson) validateContext(ctx *Context) (err error) {
	if ctx.maxPatch != -1 || ctx.currentPatch != -1 {
		return fmt.Errorf("Context must be resetted using Free() before re-using it")
	}
	return nil
}

func (r *ResolverFastJson) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (err error) {
	buf := r.getBufPair()
	defer r.freeBufPair(buf)

	responseBuf := r.getBufPair()
	defer r.freeBufPair(responseBuf)

	extractResponse(data, responseBuf, ProcessResponseConfig{ExtractGraphqlResponse: true})

	if data != nil {
		ctx.lastFetchID = initialValueID
	}

	if r.dataLoaderEnabled {
		ctx.dataLoader = r.dataloaderFactory.newDataLoader(responseBuf.Data.Bytes())
		defer func() {
			r.dataloaderFactory.freeDataLoader(ctx.dataLoader)
			ctx.dataLoader = nil
		}()
	}

	ignoreData := false

	parser := FastJsonParser.Get()
	defer FastJsonParser.Put(parser)

	value, _ := parser.ParseBytes(responseBuf.Data.Bytes())
	err = r.resolveNode(ctx, response.Data, value, buf)
	if err != nil {
		if !errors.Is(err, errNonNullableFieldValueIsNull) {
			return
		}
		ignoreData = true
	}
	if responseBuf.Errors.Len() > 0 {
		r.MergeBufPairErrors(responseBuf, buf)
	}

	return writeGraphqlResponse(buf, writer, ignoreData)
}

func (r *ResolverFastJson) ResolveGraphQLSubscription(ctx *Context, subscription *GraphQLSubscription, writer FlushWriter) (err error) {

	buf := r.getBufPair()
	err = subscription.Trigger.InputTemplate.Render(ctx, nil, buf.Data)
	if err != nil {
		return
	}
	rendered := buf.Data.Bytes()
	subscriptionInput := make([]byte, len(rendered))
	copy(subscriptionInput, rendered)
	r.freeBufPair(buf)

	c, cancel := context.WithCancel(ctx)
	defer cancel()
	resolverDone := r.ctx.Done()

	next := make(chan []byte)
	err = subscription.Trigger.Source.Start(c, subscriptionInput, next)
	if err != nil {
		if errors.Is(err, ErrUnableToResolve) {
			_, err = writer.Write([]byte(`{"errors":[{"message":"unable to resolve"}]}`))
			if err != nil {
				return err
			}
			writer.Flush()
			return nil
		}
		return err
	}

	for {
		select {
		case <-resolverDone:
			return nil
		default:
			data, ok := <-next
			if !ok {
				return nil
			}
			err = r.ResolveGraphQLResponse(ctx, subscription.Response, data, writer)
			if err != nil {
				return err
			}
			writer.Flush()
		}
	}
}

func (r *ResolverFastJson) ResolveGraphQLStreamingResponse(ctx *Context, response *GraphQLStreamingResponse, data []byte, writer FlushWriter) (err error) {

	if err := r.validateContext(ctx); err != nil {
		return err
	}

	err = r.ResolveGraphQLResponse(ctx, response.InitialResponse, data, writer)
	if err != nil {
		return err
	}
	writer.Flush()

	nextFlush := time.Now().Add(time.Millisecond * time.Duration(response.FlushInterval))

	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)

	buf.Write(literal.LBRACK)

	done := ctx.Context.Done()

Loop:
	for {
		select {
		case <-done:
			return
		default:
			patch, ok := ctx.popNextPatch()
			if !ok {
				break Loop
			}

			if patch.index > len(response.Patches)-1 {
				continue
			}

			if buf.Len() != 1 {
				buf.Write(literal.COMMA)
			}

			preparedPatch := response.Patches[patch.index]
			err = r.ResolveGraphQLResponsePatch(ctx, preparedPatch, patch.data, patch.path, patch.extraPath, buf)
			if err != nil {
				return err
			}

			now := time.Now()
			if now.After(nextFlush) {
				buf.Write(literal.RBRACK)
				_, err = writer.Write(buf.Bytes())
				if err != nil {
					return err
				}
				writer.Flush()
				buf.Reset()
				buf.Write(literal.LBRACK)
				nextFlush = time.Now().Add(time.Millisecond * time.Duration(response.FlushInterval))
			}
		}
	}

	if buf.Len() != 1 {
		buf.Write(literal.RBRACK)
		_, err = writer.Write(buf.Bytes())
		if err != nil {
			return err
		}
		writer.Flush()
	}

	return
}

func (r *ResolverFastJson) ResolveGraphQLResponsePatch(ctx *Context, patch *GraphQLResponsePatch, data, path, extraPath []byte, writer io.Writer) (err error) {

	buf := r.getBufPair()
	defer r.freeBufPair(buf)

	ctx.pathPrefix = append(path, extraPath...)

	if patch.Fetch != nil {
		set := r.getResultSet()
		defer r.freeResultSet(set)
		err = r.resolveFetch(ctx, patch.Fetch, data, set)
		if err != nil {
			return err
		}
		_, ok := set.buffers[0]
		if ok {
			r.MergeBufPairErrors(set.buffers[0], buf)
			data = set.buffers[0].Data.Bytes()
		}
	}

	parser := FastJsonParser.Get()
	defer FastJsonParser.Put(parser)
	value, _ := parser.ParseBytes(data)

	err = r.resolveNode(ctx, patch.Value, value, buf)
	if err != nil {
		return
	}

	hasErrors := buf.Errors.Len() != 0
	hasData := buf.Data.Len() != 0

	if hasErrors {
		return
	}

	if hasData {
		if hasErrors {
			err = writeSafe(err, writer, comma)
		}
		err = writeSafe(err, writer, lBrace)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, literal.OP)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, colon)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, patch.Operation)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, comma)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, literal.PATH)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, colon)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, path)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, comma)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, literal.VALUE)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, colon)
		_, err = writer.Write(buf.Data.Bytes())
		err = writeSafe(err, writer, rBrace)
	}

	return
}

func (r *ResolverFastJson) resolveEmptyArray(b *fastbuffer.FastBuffer) {
	b.WriteBytes(lBrack)
	b.WriteBytes(rBrack)
}

func (r *ResolverFastJson) resolveEmptyObject(b *fastbuffer.FastBuffer) {
	b.WriteBytes(lBrace)
	b.WriteBytes(rBrace)
}

func (r *ResolverFastJson) resolveArray(ctx *Context, array *Array, data *fastjson.Value, arrayBuf *BufPair) (err error) {
	if len(array.Path) != 0 {
		data = data.Get(array.Path...)
	}

	// handle null array response
	if data == nil || data.Type() == fastjson.TypeNull {
		if !array.Nullable {
			r.resolveEmptyArray(arrayBuf.Data)
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(arrayBuf.Data)
		return nil
	}

	// handle empty array response
	arrayItems := data.GetArray()
	if len(arrayItems) == 0 {
		r.resolveEmptyArray(arrayBuf.Data)
		return
	}

	ctx.addResponseArrayElements(array.Path)
	defer func() { ctx.removeResponseArrayLastElements(array.Path) }()

	// TODO: FIX ME
	// if array.ResolveAsynchronous && !array.Stream.Enabled && !r.dataLoaderEnabled {
	// 	return r.resolveArrayAsynchronous(ctx, array, arrayItems, arrayBuf)
	// }
	return r.resolveArraySynchronous(ctx, array, arrayItems, arrayBuf)
}

func (r *ResolverFastJson) resolveArraySynchronous(ctx *Context, array *Array, arrayItems []*fastjson.Value, arrayBuf *BufPair) (err error) {
	itemBuf := r.getBufPair()
	defer r.freeBufPair(itemBuf)

	arrayBuf.Data.WriteBytes(lBrack)
	var (
		hasPreviousItem bool
		dataWritten     int
	)
	for i := range arrayItems {
		// if array.Stream.Enabled {
		// 	if i > array.Stream.InitialBatchSize-1 {
		// 		ctx.addIntegerPathElement(i)
		// 		r.preparePatch(ctx, array.Stream.PatchIndex, nil, (*arrayItems)[i])
		// 		ctx.removeLastPathElement()
		// 		continue
		// 	}
		// }

		ctx.addIntegerPathElement(i)
		err = r.resolveNode(ctx, array.Item, arrayItems[i], itemBuf)
		ctx.removeLastPathElement()
		if err != nil {
			if errors.Is(err, errNonNullableFieldValueIsNull) && array.Nullable {
				arrayBuf.Data.Reset()
				r.resolveNull(arrayBuf.Data)
				return nil
			}
			if errors.Is(err, errTypeNameSkipped) {
				err = nil
				continue
			}
			return
		}
		dataWritten += itemBuf.Data.Len()
		r.MergeBufPairs(itemBuf, arrayBuf, hasPreviousItem)
		if !hasPreviousItem && dataWritten != 0 {
			hasPreviousItem = true
		}
	}

	arrayBuf.Data.WriteBytes(rBrack)
	return
}

func (r *ResolverFastJson) resolveArrayAsynchronous(ctx *Context, array *Array, arrayItems *[][]byte, arrayBuf *BufPair) (err error) {

	arrayBuf.Data.WriteBytes(lBrack)

	bufSlice := r.getBufPairSlice()
	defer r.freeBufPairSlice(bufSlice)

	wg := r.getWaitGroup()
	defer r.freeWaitGroup(wg)

	errCh := r.getErrChan()
	defer r.freeErrChan(errCh)

	wg.Add(len(*arrayItems))

	for i := range *arrayItems {
		itemBuf := r.getBufPair()
		*bufSlice = append(*bufSlice, itemBuf)
		// itemData := (*arrayItems)[i]
		cloned := ctx.Clone()
		go func(ctx Context, i int) {
			ctx.addPathElement([]byte(strconv.Itoa(i)))
			// if e := r.resolveNode(&ctx, array.Item, itemData, itemBuf); e != nil && !errors.Is(e, errTypeNameSkipped) {
			// 	select {
			// 	case errCh <- e:
			// 	default:
			// 	}
			// }
			ctx.Free()
			wg.Done()
		}(cloned, i)
	}

	wg.Wait()

	select {
	case err = <-errCh:
	default:
	}

	if err != nil {
		if errors.Is(err, errNonNullableFieldValueIsNull) && array.Nullable {
			arrayBuf.Data.Reset()
			r.resolveNull(arrayBuf.Data)
			return nil
		}
		return
	}

	var (
		hasPreviousItem bool
		dataWritten     int
	)
	for i := range *bufSlice {
		dataWritten += (*bufSlice)[i].Data.Len()
		r.MergeBufPairs((*bufSlice)[i], arrayBuf, hasPreviousItem)
		if !hasPreviousItem && dataWritten != 0 {
			hasPreviousItem = true
		}
	}

	arrayBuf.Data.WriteBytes(rBrack)
	return
}

func (r *ResolverFastJson) resolveNullable(nullable bool, resultData *fastbuffer.FastBuffer) error {
	if !nullable {
		return errNonNullableFieldValueIsNull
	}
	r.resolveNull(resultData)
	return nil
}

func (r *ResolverFastJson) resolveScalar(data *fastjson.Value, path []string, nullable bool, resultData *fastbuffer.FastBuffer, desiredType ...fastjson.Type) error {
	value := data.Get(path...)
	if value == nil {
		return r.resolveNullable(nullable, resultData)
	}

	var (
		validType bool
		valueType = value.Type()
	)

	for i, _ := range desiredType {
		if valueType == desiredType[i] {
			validType = true
			break
		}
	}

	if !validType {
		return r.resolveNullable(nullable, resultData)
	}

	return value.MarshalToWriter(resultData)
}

func (r *ResolverFastJson) resolveInteger(integer *Integer, data *fastjson.Value, integerBuf *BufPair) error {
	return r.resolveScalar(data, integer.Path, integer.Nullable, integerBuf.Data, fastjson.TypeNumber)
}

func (r *ResolverFastJson) resolveFloat(floatValue *Float, data *fastjson.Value, floatBuf *BufPair) error {
	return r.resolveScalar(data, floatValue.Path, floatValue.Nullable, floatBuf.Data, fastjson.TypeNumber)
}

func (r *ResolverFastJson) resolveBoolean(boolean *Boolean, data *fastjson.Value, booleanBuf *BufPair) error {
	return r.resolveScalar(data, boolean.Path, boolean.Nullable, booleanBuf.Data, fastjson.TypeFalse, fastjson.TypeTrue)
}

func (r *ResolverFastJson) resolveString(str *String, data *fastjson.Value, stringBuf *BufPair) error {
	return r.resolveScalar(data, str.Path, str.Nullable, stringBuf.Data, fastjson.TypeString)
}

func (r *ResolverFastJson) preparePatch(ctx *Context, patchIndex int, extraPath, data []byte) {
	buf := pool.BytesBuffer.Get()
	ctx.usedBuffers = append(ctx.usedBuffers, buf)
	_, _ = buf.Write(data)
	path, data := ctx.path(), buf.Bytes()
	ctx.addPatch(patchIndex, path, extraPath, data)
}

func (r *ResolverFastJson) resolveNull(b *fastbuffer.FastBuffer) {
	b.WriteBytes(null)
}

func (r *ResolverFastJson) addResolveError(ctx *Context, objectBuf *BufPair) {
	locations, path := pool.BytesBuffer.Get(), pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(locations)
	defer pool.BytesBuffer.Put(path)

	var pathBytes []byte

	locations.Write(lBrack)
	locations.Write(lBrace)
	locations.Write(quote)
	locations.Write(literalLine)
	locations.Write(quote)
	locations.Write(colon)
	locations.Write([]byte(strconv.Itoa(int(ctx.position.Line))))
	locations.Write(comma)
	locations.Write(quote)
	locations.Write(literalColumn)
	locations.Write(quote)
	locations.Write(colon)
	locations.Write([]byte(strconv.Itoa(int(ctx.position.Column))))
	locations.Write(rBrace)
	locations.Write(rBrack)

	if len(ctx.pathElements) > 0 {
		path.Write(lBrack)
		path.Write(quote)
		path.Write(bytes.Join(ctx.pathElements, quotedComma))
		path.Write(quote)
		path.Write(rBrack)

		pathBytes = path.Bytes()
	}

	objectBuf.WriteErr(unableToResolveMsg, locations.Bytes(), pathBytes, nil)
}

func (r *ResolverFastJson) resolveObject(ctx *Context, object *Object, data *fastjson.Value, objectBuf *BufPair) (err error) {
	if len(object.Path) != 0 {
		if data == nil {
			if object.Nullable {
				r.resolveNull(objectBuf.Data)
				return
			}

			r.addResolveError(ctx, objectBuf)
			return errNonNullableFieldValueIsNull
		}

		data = data.Get(object.Path...)

		if data.Type() == fastjson.TypeNull {
			if object.Nullable {
				r.resolveNull(objectBuf.Data)
				return
			}

			r.addResolveError(ctx, objectBuf)
			return errNonNullableFieldValueIsNull
		}

		ctx.addResponseElements(object.Path)
		defer ctx.removeResponseLastElements(object.Path)
	}

	var set *resultSet
	if object.Fetch != nil {
		set = r.getResultSet()
		defer r.freeResultSet(set)

		var parentObjectData []byte
		if data != nil {
			buf := pool.FastBuffer.Get()
			defer pool.FastBuffer.Put(buf)
			_ = data.MarshalToWriter(buf)
			parentObjectData = buf.Bytes()
		}

		err = r.resolveFetch(ctx, object.Fetch, parentObjectData, set)
		if err != nil {
			return
		}
		for i := range set.buffers {
			r.MergeBufPairErrors(set.buffers[i], objectBuf)
		}

		parser := FastJsonParser.Get()
		defer FastJsonParser.Put(parser)

		// TODO: defered
		data, _ = parser.ParseBytes(parentObjectData)
	}

	fieldBuf := r.getBufPair()
	defer r.freeBufPair(fieldBuf)

	responseElements := ctx.responseElements
	lastFetchID := ctx.lastFetchID

	typeNameSkip := false
	first := true

	parser := FastJsonParser.Get()
	defer FastJsonParser.Put(parser)

	for i := range object.Fields {
		var fieldData *fastjson.Value
		if set != nil && object.Fields[i].HasBuffer {
			buffer, ok := set.buffers[object.Fields[i].BufferID]
			if ok {
				fieldDataBytes := buffer.Data.Bytes()
				fieldData, _ = parser.ParseBytes(fieldDataBytes)
				ctx.resetResponsePathElements()
				ctx.lastFetchID = object.Fields[i].BufferID
			}
		} else {
			if data != nil {
				fieldData = data
			}
		}

		if object.Fields[i].OnTypeName != nil {
			typeName := fieldData.GetStringBytes("__typename")
			if !bytes.Equal(typeName, object.Fields[i].OnTypeName) {
				typeNameSkip = true
				continue
			}
		}

		if first {
			objectBuf.Data.WriteBytes(lBrace)
			first = false
		} else {
			objectBuf.Data.WriteBytes(comma)
		}
		objectBuf.Data.WriteBytes(quote)
		objectBuf.Data.WriteBytes(object.Fields[i].Name)
		objectBuf.Data.WriteBytes(quote)
		objectBuf.Data.WriteBytes(colon)
		ctx.addPathElement(object.Fields[i].Name)
		ctx.setPosition(object.Fields[i].Position)

		err = r.resolveNode(ctx, object.Fields[i].Value, fieldData, fieldBuf)
		ctx.removeLastPathElement()
		ctx.responseElements = responseElements
		ctx.lastFetchID = lastFetchID
		if err != nil {
			if errors.Is(err, errTypeNameSkipped) {
				objectBuf.Data.Reset()
				r.resolveEmptyObject(objectBuf.Data)
				return nil
			}
			if errors.Is(err, errNonNullableFieldValueIsNull) {
				objectBuf.Data.Reset()
				r.MergeBufPairErrors(fieldBuf, objectBuf)

				if object.Nullable {
					r.resolveNull(objectBuf.Data)
					return nil
				}

				// if fied is of object type than we should not add resolve error here
				if _, ok := object.Fields[i].Value.(*Object); !ok {
					r.addResolveError(ctx, objectBuf)
				}
			}

			return
		}
		r.MergeBufPairs(fieldBuf, objectBuf, false)
	}
	if first {
		if typeNameSkip && !object.Nullable {
			return errTypeNameSkipped
		}
		if !object.Nullable {
			r.addResolveError(ctx, objectBuf)
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(objectBuf.Data)
		return
	}
	objectBuf.Data.WriteBytes(rBrace)
	return
}

func (r *ResolverFastJson) freeResultSet(set *resultSet) {
	for i := range set.buffers {
		set.buffers[i].Reset()
		r.bufPairPool.Put(set.buffers[i])
		delete(set.buffers, i)
	}
	r.resultSetPool.Put(set)
}

func (r *ResolverFastJson) resolveFetch(ctx *Context, fetch Fetch, data []byte, set *resultSet) (err error) {

	switch f := fetch.(type) {
	case *SingleFetch:
		preparedInput := r.getBufPair()
		defer r.freeBufPair(preparedInput)
		err = r.prepareSingleFetch(ctx, f, data, set, preparedInput.Data)
		if err != nil {
			return err
		}
		err = r.resolveSingleFetch(ctx, f, preparedInput.Data, set.buffers[f.BufferId])
	case *BatchFetch:
		preparedInput := r.getBufPair()
		defer r.freeBufPair(preparedInput)
		err = r.prepareSingleFetch(ctx, f.Fetch, data, set, preparedInput.Data)
		if err != nil {
			return err
		}
		err = r.resolveBatchFetch(ctx, f, preparedInput.Data, set.buffers[f.Fetch.BufferId])
	case *ParallelFetch:
		err = r.resolveParallelFetch(ctx, f, data, set)
	}
	return
}

func (r *ResolverFastJson) resolveParallelFetch(ctx *Context, fetch *ParallelFetch, data []byte, set *resultSet) (err error) {
	preparedInputs := r.getBufPairSlice()
	defer r.freeBufPairSlice(preparedInputs)

	resolvers := make([]func() error, 0, len(fetch.Fetches))

	wg := r.getWaitGroup()
	defer r.freeWaitGroup(wg)

	for i := range fetch.Fetches {
		wg.Add(1)
		switch f := fetch.Fetches[i].(type) {
		case *SingleFetch:
			preparedInput := r.getBufPair()
			err = r.prepareSingleFetch(ctx, f, data, set, preparedInput.Data)
			if err != nil {
				return err
			}
			*preparedInputs = append(*preparedInputs, preparedInput)
			buf := set.buffers[f.BufferId]
			resolvers = append(resolvers, func() error {
				return r.resolveSingleFetch(ctx, f, preparedInput.Data, buf)
			})
		case *BatchFetch:
			preparedInput := r.getBufPair()
			err = r.prepareSingleFetch(ctx, f.Fetch, data, set, preparedInput.Data)
			if err != nil {
				return err
			}
			*preparedInputs = append(*preparedInputs, preparedInput)
			buf := set.buffers[f.Fetch.BufferId]
			resolvers = append(resolvers, func() error {
				return r.resolveBatchFetch(ctx, f, preparedInput.Data, buf)
			})
		}
	}

	for _, resolver := range resolvers {
		go func(r func() error) {
			_ = r()
			wg.Done()
		}(resolver)
	}

	wg.Wait()

	return
}

func (r *ResolverFastJson) prepareSingleFetch(ctx *Context, fetch *SingleFetch, data []byte, set *resultSet, preparedInput *fastbuffer.FastBuffer) (err error) {
	err = fetch.InputTemplate.Render(ctx, data, preparedInput)
	buf := r.getBufPair()
	set.buffers[fetch.BufferId] = buf
	return
}

func (r *ResolverFastJson) resolveBatchFetch(ctx *Context, fetch *BatchFetch, preparedInput *fastbuffer.FastBuffer, buf *BufPair) error {
	if r.dataLoaderEnabled {
		return ctx.dataLoader.LoadBatch(ctx, fetch, buf)
	}

	if err := r.fetcher.FetchBatch(ctx, fetch, []*fastbuffer.FastBuffer{preparedInput}, []*BufPair{buf}); err != nil {
		return err
	}

	return nil
}

func (r *ResolverFastJson) resolveSingleFetch(ctx *Context, fetch *SingleFetch, preparedInput *fastbuffer.FastBuffer, buf *BufPair) error {
	if r.dataLoaderEnabled {
		return ctx.dataLoader.Load(ctx, fetch, buf)
	}

	return r.fetcher.Fetch(ctx, fetch, preparedInput, buf)
}

func (r *ResolverFastJson) MergeBufPairs(from, to *BufPair, prefixDataWithComma bool) {
	r.MergeBufPairData(from, to, prefixDataWithComma)
	r.MergeBufPairErrors(from, to)
}

func (r *ResolverFastJson) MergeBufPairData(from, to *BufPair, prefixDataWithComma bool) {
	if !from.HasData() {
		return
	}
	if prefixDataWithComma {
		to.Data.WriteBytes(comma)
	}
	to.Data.WriteBytes(from.Data.Bytes())
	from.Data.Reset()
}

func (r *ResolverFastJson) MergeBufPairErrors(from, to *BufPair) {
	if !from.HasErrors() {
		return
	}
	if to.HasErrors() {
		to.Errors.WriteBytes(comma)
	}
	to.Errors.WriteBytes(from.Errors.Bytes())
	from.Errors.Reset()
}

func (r *ResolverFastJson) freeBufPair(pair *BufPair) {
	pair.Data.Reset()
	pair.Errors.Reset()
	r.bufPairPool.Put(pair)
}

func (r *ResolverFastJson) getResultSet() *resultSet {
	return r.resultSetPool.Get().(*resultSet)
}

func (r *ResolverFastJson) getBufPair() *BufPair {
	return r.bufPairPool.Get().(*BufPair)
}

func (r *ResolverFastJson) getBufPairSlice() *[]*BufPair {
	return r.bufPairSlicePool.Get().(*[]*BufPair)
}

func (r *ResolverFastJson) freeBufPairSlice(slice *[]*BufPair) {
	for i := range *slice {
		r.freeBufPair((*slice)[i])
	}
	*slice = (*slice)[:0]
	r.bufPairSlicePool.Put(slice)
}

func (r *ResolverFastJson) getErrChan() chan error {
	return r.errChanPool.Get().(chan error)
}

func (r *ResolverFastJson) freeErrChan(ch chan error) {
	r.errChanPool.Put(ch)
}

func (r *ResolverFastJson) getWaitGroup() *sync.WaitGroup {
	return r.waitGroupPool.Get().(*sync.WaitGroup)
}

func (r *ResolverFastJson) freeWaitGroup(wg *sync.WaitGroup) {
	r.waitGroupPool.Put(wg)
}
