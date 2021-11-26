package benchmarking

import (
	"github.com/francoispqt/gojay"

	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

type goJayInput struct {
	body goJayBody
}

func (u *goJayInput) UnmarshalJSONObject(dec *gojay.Decoder, key string) error {
	switch key {
	case "body":
		err := dec.DecodeObject(&u.body)
		return err
	}
	return nil
}
func (u *goJayInput) NKeys() int {
	return 4
}

type goJayBody struct {
	variables goJayVariables
}

func (u *goJayBody) UnmarshalJSONObject(dec *gojay.Decoder, key string) error {
	switch key {
	case "variables":
		err := dec.DecodeObject(&u.variables)
		return err
	}
	return nil
}
func (u *goJayBody) NKeys() int {
	return 2
}

type goJayVariables struct {
	representations goJayRepresentationsArr
}

func (u *goJayVariables) UnmarshalJSONObject(dec *gojay.Decoder, key string) error {
	switch key {
	case "representations":
		err := dec.DecodeArray(&u.representations)
		return err
	}
	return nil
}
func (u *goJayVariables) NKeys() int {
	return 1
}

type goJayRepresentationsArr []goJayRepresentations

func (t *goJayRepresentationsArr) UnmarshalJSONArray(dec *gojay.Decoder) error {
	reps := make(goJayRepresentations)
	if err := dec.DecodeObject(&reps); err != nil {
		return err
	}
	*t = append(*t, reps)
	return nil
}

type goJayRepresentations map[string]string

// Implementing Unmarshaler
func (m goJayRepresentations) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {
	str := ""
	err := dec.String(&str)
	if err != nil {
		return err
	}
	m[k] = str
	return nil
}

// we return 0, it tells the Decoder to decode all keys
func (m goJayRepresentations) NKeys() int {
	return 0
}

func (m goJayRepresentations) MarshalJSONObject(enc *gojay.Encoder) {
	for k, v := range m {
		enc.StringKey(k, v)
	}
}

func (m goJayRepresentations) IsNil() bool {
	return m == nil
}

func GojayExtraction(inputs [][]byte) (variables []byte, err error) {
	variablesBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(variablesBuf)

	variablesBuf.WriteBytes(literal.LBRACK)

	for i := range inputs {
		if i != 0 {
			variablesBuf.WriteBytes(literal.COMMA)
		}

		// any := jsoniter.Get(inputs[i], representationPath[0], representationPath[1], representationPath[2], 0)
		// variablesBuf.WriteString(any.ToString())
		input := goJayInput{}
		_ = gojay.UnmarshalJSONObject(inputs[i], &input)

		for j := 0; j < len(input.body.variables.representations); j++ {
			reps, err := gojay.Marshal(input.body.variables.representations[j])
			if err != nil {
				return nil, err
			}

			variablesBuf.WriteBytes(reps)
		}

	}

	// variablesBuf.WriteBytes(stream.Buffer())
	variablesBuf.WriteBytes(literal.RBRACK)

	representationJson := variablesBuf.Bytes()
	representationJsonCopy := make([]byte, len(representationJson))
	copy(representationJsonCopy, representationJson)

	return representationJsonCopy, nil
}

// 		var inputContent = []byte(`{
//     "method": "POST",
//     "url": "http://localhost:4003",
//     "body": {
//         "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}",
//         "variables": {
//             "representations": [
//                 {
//                     "upc": "top-1",
//                     "__typename": "Product"
//                 }
//             ]
//         }
//     }
// }`)
