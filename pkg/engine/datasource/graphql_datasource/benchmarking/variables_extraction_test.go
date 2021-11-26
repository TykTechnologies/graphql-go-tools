package benchmarking

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

var inputCount = 1000

var inputContent = []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`)
var vars = []byte(`{"upc":"top-1","__typename":"Product"}`)

func buildInputs() (inputs [][]byte, expected []byte) {
	inputs = make([][]byte, 0, inputCount)

	buf := bytes.Buffer{}
	buf.Write([]byte(`[`))

	for i := 0; i < inputCount; i++ {
		inputs = append(inputs, inputContent)
		if i != 0 {
			buf.Write([]byte(`,`))
		}
		buf.Write(vars)
	}

	buf.Write([]byte(`]`))

	return inputs, buf.Bytes()
}

func TestExtraction(t *testing.T) {
	inputs, expected := buildInputs()

	actual, _ := OriginalExtractionParallel(inputs)

	assert.Equal(t, expected, actual)
}

func BenchmarkOriginalExtraction(b *testing.B) {
	inputs, expected := buildInputs()

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			actual, _ := OriginalExtraction(inputs)
			if !bytes.Equal(expected, actual) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), string(actual))
			}
		}
	})
}

func BenchmarkOriginalExtractionModified(b *testing.B) {
	inputs, expected := buildInputs()

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			actual, _ := OriginalExtractionModified(inputs)
			if !bytes.Equal(expected, actual) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), string(actual))
			}
		}
	})
}

func BenchmarkOriginalExtractionParallel(b *testing.B) {
	inputs, expected := buildInputs()

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			actual, _ := OriginalExtractionParallel(inputs)
			if !bytes.Equal(expected, actual) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), string(actual))
			}
		}
	})
}

func BenchmarkFastJsonExtraction(b *testing.B) {
	inputs, expected := buildInputs()

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			actual, _ := FastJsonExtraction(inputs)
			if !bytes.Equal(expected, actual) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), string(actual))
			}
		}
	})
}

func BenchmarkJsoniterExtraction(b *testing.B) {
	inputs, expected := buildInputs()

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			actual, _ := JsoniterExtraction(inputs)
			if !bytes.Equal(expected, actual) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), string(actual))
			}
		}
	})
}

func BenchmarkJsoniterGetExtraction(b *testing.B) {
	inputs, expected := buildInputs()

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = JsoniterGetExtraction(inputs)
			actual, _ := JsoniterGetExtraction(inputs)
			if !bytes.Equal(expected, actual) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), string(actual))
			}
		}
	})
}

func BenchmarkGojayExtraction(b *testing.B) {
	inputs, expected := buildInputs()

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = GojayExtraction(inputs)
			// if !bytes.Equal(expected, actual) {
			// 	b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), string(actual))
			// }
		}
	})
}
