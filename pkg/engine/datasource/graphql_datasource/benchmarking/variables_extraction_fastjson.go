package benchmarking

import (
	"github.com/valyala/fastjson"

	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

var parserPool = fastjson.ParserPool{}

func FastJsonExtraction(inputs [][]byte) (variables []byte, err error) {
	parser := parserPool.Get()
	defer parserPool.Put(parser)

	variablesBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(variablesBuf)

	var (
		variablesIdx int
	)

	variablesBuf.WriteBytes(literal.LBRACK)

	inputValue := make([]byte, 0, 1024)

	for i := range inputs {
		jsonValue, err := parser.ParseBytes(inputs[i])
		if err != nil {
			return nil, err
		}

		variablesValue := jsonValue.GetArray(representationPath...)

		for j := 0; j < len(variablesValue); j++ {
			if i != 0 {
				variablesBuf.WriteBytes(literal.COMMA)
			}
			inputValue = variablesValue[j].MarshalTo(inputValue)
			variablesBuf.WriteBytes(inputValue)
			inputValue = inputValue[:0]

			variablesIdx++
		}
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
