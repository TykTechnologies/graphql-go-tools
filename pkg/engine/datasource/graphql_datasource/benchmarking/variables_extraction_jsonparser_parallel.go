package benchmarking

import (
	"bytes"
	"sync"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

type job struct {
	item   int
	input  *[]byte
	result []byte
	err    error
}

func (j *job) reset() {
	j.input = nil
	j.err = nil
	j.result = j.result[:0]
}

func OriginalExtractionParallel(inputs [][]byte) (variables []byte, err error) {
	variablesBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(variablesBuf)

	variablesBuf.WriteBytes(literal.LBRACK)

	resultsReps := make([][]byte, len(inputs))

	jobsPool := sync.Pool{New: func() interface{} {
		return &job{
			result: make([]byte, 0, 1024),
		}
	}}

	workerCount := 10

	jobs := make(chan *job, workerCount)
	results := make(chan *job, workerCount)

	var wg sync.WaitGroup
	wg.Add(len(inputs))

	for i := 0; i < workerCount; i++ {
		go func() {
			for j := range jobs {
				_, err = jsonparser.ArrayEach(*j.input, func(value []byte, _ jsonparser.ValueType, _ int, _ error) {
					j.result = j.result[:0]
					j.result = append(j.result, value...)
				}, representationPath...)
				if err != nil {
					j.err = err
				}
				results <- j
			}
		}()
	}
	for i := 0; i < workerCount; i++ {
		go func() {
			for result := range results {
				res := make([]byte, 0, len(result.result))
				res = append(res, result.result...)
				resultsReps[result.item] = res

				result.reset()
				jobsPool.Put(result)
				wg.Done()
			}
		}()
	}

	go func() {
		for i := range inputs {
			j := jobsPool.Get().(*job)
			j.item = i
			j.input = &inputs[i]

			jobs <- j
		}
		close(jobs)
	}()

	wg.Wait()
	close(results)

	variablesBuf.WriteBytes(bytes.Join(resultsReps, literal.COMMA))

	variablesBuf.WriteBytes(literal.RBRACK)

	representationJson := variablesBuf.Bytes()
	representationJsonCopy := make([]byte, len(representationJson))
	copy(representationJsonCopy, representationJson)

	return representationJsonCopy, nil
}
