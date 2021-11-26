package benchmarking

import (
	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

var representationPath = []string{"body", "variables", "representations"}

func OriginalExtraction(inputs [][]byte) (variables []byte, err error) {
	variablesBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(variablesBuf)

	var (
		variablesIdx int
	)

	variablesBuf.WriteBytes(literal.LBRACK)

	for i := range inputs {
		inputVariables, _, _, err := jsonparser.Get(inputs[i], representationPath...)
		if err != nil {
			return nil, err
		}

		_, err = jsonparser.ArrayEach(inputVariables, func(value []byte, _ jsonparser.ValueType, _ int, _ error) {
			if variablesBuf.Len() != 1 {
				variablesBuf.WriteBytes(literal.COMMA)
			}
			variablesBuf.WriteBytes(value)
			variablesIdx++
		})
		if err != nil {
			return nil, err
		}
	}

	variablesBuf.WriteBytes(literal.RBRACK)

	representationJson := variablesBuf.Bytes()
	representationJsonCopy := make([]byte, len(representationJson))
	copy(representationJsonCopy, representationJson)

	return representationJsonCopy, nil
}

func OriginalExtractionModified(inputs [][]byte) (variables []byte, err error) {
	variablesBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(variablesBuf)

	// grow := make([]byte, 1024*len(inputs))
	// variablesBuf.WriteBytes(grow)
	// variablesBuf.Reset()

	var (
		variablesIdx int
	)

	variablesBuf.WriteBytes(literal.LBRACK)

	for i := range inputs {
		_, err = jsonparser.ArrayEach(inputs[i], func(value []byte, _ jsonparser.ValueType, _ int, _ error) {
			if variablesBuf.Len() != 1 {
				variablesBuf.WriteBytes(literal.COMMA)
			}
			variablesBuf.WriteBytes(value)
			variablesIdx++
		}, representationPath...)
		if err != nil {
			return nil, err
		}
	}

	variablesBuf.WriteBytes(literal.RBRACK)

	representationJson := variablesBuf.Bytes()
	representationJsonCopy := make([]byte, len(representationJson))
	copy(representationJsonCopy, representationJson)

	return representationJsonCopy, nil
}
