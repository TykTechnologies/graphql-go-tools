package benchmarking

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

var streamPool = jsoniter.NewStream(jsoniter.ConfigFastest, nil, 1024).Pool()
var iteratorPool = jsoniter.NewIterator(jsoniter.ConfigFastest).Pool()

func JsoniterExtraction(inputs [][]byte) (variables []byte, err error) {
	parser := parserPool.Get()
	defer parserPool.Put(parser)

	variablesBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(variablesBuf)

	var (
		variablesIdx int
	)

	variablesBuf.WriteBytes(literal.LBRACK)

	iter := iteratorPool.BorrowIterator(nil)
	defer iteratorPool.ReturnIterator(iter)

	for i := range inputs {
		// stream := streamPool.BorrowStream(nil)
		if i != 0 {
			variablesBuf.WriteBytes(literal.COMMA)
		}

		iter.ResetBytes(inputs[i])

		for iterateInput := true; iterateInput; {
			iterateInput = iter.ReadObjectCB(func(iterator *jsoniter.Iterator, s string) bool {
				if s != "body" {
					iterator.Skip()
					return true
				}

				for iterateBody := true; iterateBody; {
					iterateBody = iter.ReadObjectCB(func(iterator *jsoniter.Iterator, s string) bool {
						if s != "variables" {
							iterator.Skip()
							return true
						}

						iter.ReadObjectCB(func(iterator *jsoniter.Iterator, s string) bool {
							for iterateRepresentations := true; iterateRepresentations; {
								iterateRepresentations = iter.ReadArrayCB(func(iterator *jsoniter.Iterator) bool {
									smth := iterator.ReadAny()
									// smth.WriteTo(stream)
									variablesBuf.WriteString(smth.ToString())
									variablesIdx++

									next := iter.WhatIsNext()
									return next != jsoniter.InvalidValue
								})
							}

							next := iter.WhatIsNext()
							return next != jsoniter.InvalidValue
						})

						return false
					})
				}

				return false

			})
		}

		// variablesBuf.WriteBytes(stream.Buffer())
		// streamPool.ReturnStream(stream)
	}

	variablesBuf.WriteBytes(literal.RBRACK)

	representationJson := variablesBuf.Bytes()
	representationJsonCopy := make([]byte, len(representationJson))
	copy(representationJsonCopy, representationJson)

	return representationJsonCopy, nil
}

func JsoniterGetExtraction(inputs [][]byte) (variables []byte, err error) {
	parser := parserPool.Get()
	defer parserPool.Put(parser)

	variablesBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(variablesBuf)

	variablesBuf.WriteBytes(literal.LBRACK)

	for i := range inputs {
		if i != 0 {
			variablesBuf.WriteBytes(literal.COMMA)
		}

		any := jsoniter.Get(inputs[i], representationPath[0], representationPath[1], representationPath[2], 0)
		variablesBuf.WriteString(any.ToString())
	}

	// variablesBuf.WriteBytes(stream.Buffer())
	variablesBuf.WriteBytes(literal.RBRACK)

	representationJson := variablesBuf.Bytes()
	representationJsonCopy := make([]byte, len(representationJson))
	copy(representationJsonCopy, representationJson)

	return representationJsonCopy, nil
}
