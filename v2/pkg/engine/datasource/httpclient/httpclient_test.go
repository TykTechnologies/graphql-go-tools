package httpclient

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/quotes"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/lexer/literal"
)

func TestHttpClient(t *testing.T) {
	in := SetInputMethod(nil, literal.HTTP_METHOD_GET)
	assert.Equal(t, `{"method":"GET"}`, string(in))

	in = SetInputMethod(nil, quotes.WrapBytes(literal.HTTP_METHOD_POST))
	assert.Equal(t, `{"method":"POST"}`, string(in))

	in = SetInputURL(nil, []byte("foo.bar.com"))
	assert.Equal(t, `{"url":"foo.bar.com"}`, string(in))

	in = SetInputURL(nil, []byte("\"foo.bar.com\""))
	assert.Equal(t, `{"url":"foo.bar.com"}`, string(in))

	in = SetInputQueryParams(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"query_params":{"foo":"bar"}}`, string(in))

	in = SetInputHeader(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"header":{"foo":"bar"}}`, string(in))

	in = SetInputHeader(nil, []byte(`[true]`))
	assert.Equal(t, `{"header":[true]}`, string(in))

	in = SetInputHeader(nil, []byte(`[null]`))
	assert.Equal(t, `{"header":[null]}`, string(in))

	in = SetInputHeader(nil, []byte(`["str"]`))
	assert.Equal(t, `{"header":["str"]}`, string(in))

	in = SetInputBody(nil, []byte(`{"foo":"bar"}`))
	assert.Equal(t, `{"body":{"foo":"bar"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{"foo":"bar"}`), "variables")
	assert.Equal(t, `{"body":{"variables":{"foo":"bar"}}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`query { foo }`), "query")
	assert.Equal(t, `{"body":{"query":"query { foo }"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{ foo }`), "query")
	assert.Equal(t, `{"body":{"query":"{ foo }"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{foo}`), "query")
	assert.Equal(t, `{"body":{"query":"{foo}"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{`), "query")
	assert.Equal(t, `{"body":{"query":"{"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{topProducts {upc name price}}}`), "query")
	assert.Equal(t, `{"body":{"query":"{topProducts {upc name price}}}"}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`$$0$$`), "variables.foo")
	assert.Equal(t, `{"body":{"variables":{"foo":$$0$$}}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`"$$0$$"`), "variables.foo")
	assert.Equal(t, `{"body":{"variables":{"foo":"$$0$$"}}}`, string(in))

	in = SetInputBodyWithPath(nil, []byte(`{"bar":$$0$$}`), "variables.foo")
	assert.Equal(t, `{"body":{"variables":{"foo":{"bar":$$0$$}}}}`, string(in))
}

func TestApplyHeaderModifier(t *testing.T) {
	setAuth := func(value string) func(http.Header) {
		return func(header http.Header) { header.Set("Authorization", value) }
	}

	t.Run("nil modifier returns input unchanged", func(t *testing.T) {
		in := []byte(`{"method":"GET","header":{"Authorization":["old"]}}`)
		out := ApplyHeaderModifier(in, nil)
		assert.Equal(t, string(in), string(out))
	})

	t.Run("no existing header key creates one", func(t *testing.T) {
		in := []byte(`{"method":"GET"}`)
		out := ApplyHeaderModifier(in, setAuth("new"))
		assert.Equal(t, `{"header":{"Authorization":["new"]},"method":"GET"}`, string(out))
	})

	t.Run("non-object header value is a no-op", func(t *testing.T) {
		in := []byte(`{"method":"GET","header":[1,2,3]}`)
		out := ApplyHeaderModifier(in, setAuth("new"))
		assert.Equal(t, `{"method":"GET","header":[1,2,3]}`, string(out))
	})

	t.Run("existing header object is rewritten by the modifier", func(t *testing.T) {
		in := []byte(`{"method":"GET","header":{"Authorization":["old"]}}`)
		out := ApplyHeaderModifier(in, setAuth("new"))
		assert.Equal(t, `{"method":"GET","header":{"Authorization":["new"]}}`, string(out))
	})

	t.Run("modifier can add a key alongside existing ones", func(t *testing.T) {
		in := []byte(`{"header":{"Authorization":["old"]}}`)
		out := ApplyHeaderModifier(in, func(header http.Header) {
			header.Set("X-Trace", "abc")
		})
		assert.Equal(t, `{"header":{"Authorization":["old"],"X-Trace":["abc"]}}`, string(out))
	})
}

func TestMergeInputHeader(t *testing.T) {
	t.Run("empty upstream headers is a no-op", func(t *testing.T) {
		in := []byte(`{"method":"GET"}`)
		assert.Equal(t, `{"method":"GET"}`, string(MergeInputHeader(in, nil)))
		assert.Equal(t, `{"method":"GET"}`, string(MergeInputHeader(in, http.Header{})))
	})

	t.Run("no existing header key creates one", func(t *testing.T) {
		in := []byte(`{"method":"GET"}`)
		out := MergeInputHeader(in, http.Header{"Authorization": []string{"secret"}})
		assert.Equal(t, `{"header":{"Authorization":["secret"]},"method":"GET"}`, string(out))
	})

	t.Run("matching keys are overwritten, other keys are preserved", func(t *testing.T) {
		in := []byte(`{"header":{"Authorization":["old"],"X-Client":["alpha"]}}`)
		out := MergeInputHeader(in, http.Header{"Authorization": []string{"new"}})
		assert.Equal(t, `{"header":{"Authorization":["new"],"X-Client":["alpha"]}}`, string(out))
	})

	t.Run("non-object existing header value is left untouched", func(t *testing.T) {
		in := []byte(`{"header":[1,2,3]}`)
		out := MergeInputHeader(in, http.Header{"Authorization": []string{"secret"}})
		assert.Equal(t, `{"header":[1,2,3]}`, string(out))
	})
}

// TestFinalizeInputHeaders covers FinalizeInputHeaders directly - behavior that
// ApplyHeaderModifier/MergeInputHeader can't exercise on their own since each
// only ever passes one of modifier/upstreamHeaders (never both), and both
// wrappers swallow the error return instead of surfacing it.
func TestFinalizeInputHeaders(t *testing.T) {
	t.Run("nil modifier and empty upstream headers returns input unchanged", func(t *testing.T) {
		in := []byte(`{"method":"GET","header":{"Authorization":["old"]}}`)

		out, err := FinalizeInputHeaders(in, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, string(in), string(out))

		out, err = FinalizeInputHeaders(in, nil, http.Header{})
		require.NoError(t, err)
		assert.Equal(t, string(in), string(out))
	})

	t.Run("modifier normalizes scalar and mixed-case values", func(t *testing.T) {
		in := []byte(`{"header":{"authorization":"first","Authorization":["second","third"]}}`)
		out, err := FinalizeInputHeaders(in, func(header http.Header) {
			header.Add("AUTHORIZATION", "current")
		}, nil)
		require.NoError(t, err)

		var decoded struct {
			Header http.Header `json:"header"`
		}
		require.NoError(t, json.Unmarshal(out, &decoded))
		assert.ElementsMatch(t, []string{"first", "second", "third", "current"}, decoded.Header.Values("Authorization"))
	})

	t.Run("upstream headers take final precedence over the modifier, other modifier keys survive", func(t *testing.T) {
		in := []byte(`{"header":{}}`)
		out, err := FinalizeInputHeaders(in, func(header http.Header) {
			header.Set("Authorization", "from-modifier")
			header.Set("X-Trace", "abc")
		}, http.Header{"Authorization": {"from-upstream"}})
		require.NoError(t, err)

		var decoded struct {
			Header http.Header `json:"header"`
		}
		require.NoError(t, json.Unmarshal(out, &decoded))
		assert.Equal(t, []string{"from-upstream"}, decoded.Header.Values("Authorization"),
			"upstream headers must take final precedence over whatever the modifier set")
		assert.Equal(t, []string{"abc"}, decoded.Header.Values("X-Trace"),
			"keys the modifier set that upstream headers don't mention must survive")
	})

	t.Run("malformed header value returns an error", func(t *testing.T) {
		_, err := FinalizeInputHeaders([]byte(`{"header":{"Authorization":42}}`), func(http.Header) {}, nil)
		assert.Error(t, err)
	})
}

// TestApplyHeaderModifierCreatesMissingHeader guards against the case where a
// datasource's input has no "header" key at all yet - ApplyHeaderModifier must
// still be able to add one, not silently no-op.
func TestApplyHeaderModifierCreatesMissingHeader(t *testing.T) {
	in := []byte(`{"method":"GET"}`)
	out := ApplyHeaderModifier(in, func(header http.Header) {
		header.Set("Authorization", "new")
	})

	headerBytes, dataType, _, err := jsonparser.Get(out, HEADER)
	require.NoError(t, err, "ApplyHeaderModifier must create a \"header\" key when none exists yet, got %s", out)
	require.Equal(t, jsonparser.Object, dataType)

	var decoded struct {
		Header http.Header `json:"header"`
	}
	require.NoError(t, json.Unmarshal(headerBytes, &decoded.Header))
	assert.Equal(t, []string{"new"}, decoded.Header.Values("Authorization"))
}

// TestApplyHeaderModifierAppliesDespiteScalarHeaderValue guards against a
// datasource input whose "header" object mixes scalar and array-shaped values -
// a realistic shape for hand-built or third-party datasource configs. The
// modifier must still be applied to the other keys, rather than the whole
// operation being silently abandoned because one value didn't parse as
// []string.
func TestApplyHeaderModifierAppliesDespiteScalarHeaderValue(t *testing.T) {
	in := []byte(`{"header":{"Authorization":"first"}}`)
	out := ApplyHeaderModifier(in, func(header http.Header) {
		header.Set("X-Trace", "abc")
	})
	assert.Contains(t, string(out), `"X-Trace"`,
		"a scalar-valued existing header entry must not silently prevent the modifier from being applied, got %s", out)
}

// TestApplyHeaderModifierCanonicalizesHeaderKeyCasing guards against header
// keys surviving as raw, non-canonical JSON object keys - e.g. "authorization"
// and "Authorization" must be treated as the same header, not two independent
// entries.
func TestApplyHeaderModifierCanonicalizesHeaderKeyCasing(t *testing.T) {
	in := []byte(`{"header":{"authorization":["legacy"]}}`)
	out := ApplyHeaderModifier(in, func(header http.Header) {
		header.Add("Authorization", "current")
	})

	headerBytes, _, _, err := jsonparser.Get(out, HEADER)
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(headerBytes, &raw))

	_, hasLowercase := raw["authorization"]
	assert.False(t, hasLowercase, `header keys must be canonicalized; found a raw "authorization" key in %s`, out)

	var values []string
	require.NoError(t, json.Unmarshal(raw["Authorization"], &values))
	assert.Equal(t, []string{"legacy", "current"}, values)
}

func TestHttpClientDo(t *testing.T) {

	runTest := func(ctx context.Context, input []byte, expectedOutput string) func(t *testing.T) {
		return func(t *testing.T) {
			out := &bytes.Buffer{}
			err := Do(http.DefaultClient, ctx, input, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, out.String())
		}
	}

	background := context.Background()

	t.Run("simple get", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := httputil.DumpRequest(r, true)
			assert.NoError(t, err)
			_, err = w.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("query params simple", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fooValues := r.URL.Query()["foo"]
			assert.Len(t, fooValues, 1)
			assert.Equal(t, fooValues[0], "bar")
			_, err := w.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		input = SetInputQueryParams(input, []byte(`[{"name":"foo","value":"bar"}]`))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("query params multiple", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fooValues := r.URL.Query()["foo"]
			assert.Len(t, fooValues, 2)
			assert.Equal(t, fooValues[0], "bar")
			assert.Equal(t, fooValues[1], "baz")

			yearValues := r.URL.Query()["year"]
			assert.Len(t, yearValues, 1)
			assert.Equal(t, yearValues[0], "2020")

			_, err := w.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		input = SetInputQueryParams(input, []byte(`[{"name":"foo","value":"bar"},{"name":"foo","value":"baz"},{"name":"year","value":"2020"}]`))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("query params multiple as array", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fooValues := r.URL.Query()["foo"]
			assert.Len(t, fooValues, 2)
			assert.Equal(t, fooValues[0], "bar")
			assert.Equal(t, fooValues[1], "baz")
			_, err := w.Write([]byte("ok"))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		input = SetInputQueryParams(input, []byte(`[{"name":"foo","value":["bar","baz"]}]`))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("post", func(t *testing.T) {
		body := []byte(`{"foo":"bar"}`)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("ok"))
			assert.NoError(t, err)
			actualBody, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.Equal(t, string(body), string(actualBody))
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("POST"))
		input = SetInputBody(input, body)
		input = SetInputURL(input, []byte(server.URL))
		t.Run("net", runTest(background, input, `ok`))
	})

	t.Run("compression", func(t *testing.T) {
		t.Run("gzip", func(t *testing.T) {
			body := []byte(`{"foo":"bar"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Contains(t, r.Header.Values(AcceptEncodingHeader), EncodingGzip)
				actualBody, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, string(body), string(actualBody))
				gzipWriter := gzip.NewWriter(w)
				defer gzipWriter.Close()
				w.Header().Set("Content-Encoding", EncodingGzip)
				_, err = gzipWriter.Write([]byte("ok"))
				assert.NoError(t, err)
			}))
			defer server.Close()
			var input []byte
			input = SetInputMethod(input, []byte("POST"))
			input = SetInputBody(input, body)
			input = SetInputURL(input, []byte(server.URL))
			t.Run("net", runTest(background, input, `ok`))
		})

		t.Run("brotli", func(t *testing.T) {
			body := []byte(`{"foo":"bar"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Contains(t, r.Header.Values(AcceptEncodingHeader), EncodingBrotli)
				actualBody, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, string(body), string(actualBody))
				brWriter := brotli.NewWriter(w)
				defer brWriter.Close()
				w.Header().Set(ContentEncodingHeader, EncodingBrotli)
				_, err = brWriter.Write([]byte("ok"))
				assert.NoError(t, err)
			}))
			defer server.Close()
			var input []byte
			input = SetInputMethod(input, []byte("POST"))
			input = SetInputBody(input, body)
			input = SetInputURL(input, []byte(server.URL))
			t.Run("net", runTest(background, input, `ok`))
		})
	})

	t.Run("redact sensitive headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := httputil.DumpRequest(r, true)
			assert.NoError(t, err)
			w.Header().Set("Authorization", "test")
			_, err = w.Write([]byte(`{"extensions": {"trace": {}}"}`))
			assert.NoError(t, err)
		}))
		defer server.Close()
		var input []byte
		input = SetInputMethod(input, []byte("GET"))
		input = SetInputURL(input, []byte(server.URL))
		input, err := sjson.SetBytes(input, TRACE, true)
		assert.NoError(t, err)
		out := &bytes.Buffer{}
		err = Do(http.DefaultClient, context.Background(), input, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), `"Authorization":["****"]`)
	})
}

// benchFinalizeInputSink keeps the compiler from eliding the benchmarked call.
var benchFinalizeInputSink []byte

// benchFinalizeInput mirrors what the loader hands to FinalizeInputHeaders: a
// fully rendered fetch input - query body and all - carrying a header object of
// the size a real datasource config produces.
func benchFinalizeInput() []byte {
	return []byte(`{"method":"POST","url":"http://localhost:4001/graphql",` +
		`"body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price reviews {body author {username}}}}}",` +
		`"variables":{"representations":[{"__typename":"Product","upc":"top-1"},{"__typename":"Product","upc":"top-2"}]}},` +
		`"header":{"Authorization":["Bearer abcdefghijklmnop"],"X-Request-Id":["7f3c9a12"],` +
		`"X-Forwarded-For":["10.0.0.1"],"Content-Type":["application/json"],"X-Tyk-Api-Name":["reviews"]}}`)
}

// BenchmarkFinalizeInputHeaders measures the per-fetch cost of finalizing the
// headers. The gateway always installs a header modifier, so "modifier only" is
// the case that matters in production; "nothing to do" measures the early
// return every other embedder gets.
func BenchmarkFinalizeInputHeaders(b *testing.B) {
	input := benchFinalizeInput()
	upstreamHeaders := http.Header{
		"X-Upstream-Tenant": []string{"acme"},
		"Authorization":     []string{"Bearer upstream"},
	}
	// modifier mirrors the gateway's: default in what is missing, then rewrite
	// every value (tyk/internal/graphengine/graphql_go_tools_v2.go).
	modifier := func(header http.Header) {
		for key := range upstreamHeaders {
			if header.Get(key) == "" {
				header.Set(key, upstreamHeaders.Get(key))
			}
		}
		for key := range header {
			header.Set(key, header.Get(key))
		}
	}

	for _, benchmark := range []struct {
		name            string
		modifier        func(http.Header)
		upstreamHeaders http.Header
	}{
		{name: "nothing to do"},
		{name: "modifier only", modifier: modifier},
		{name: "upstream headers only", upstreamHeaders: upstreamHeaders},
		{name: "modifier and upstream headers", modifier: modifier, upstreamHeaders: upstreamHeaders},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				out, err := FinalizeInputHeaders(input, benchmark.modifier, benchmark.upstreamHeaders)
				if err != nil {
					b.Fatal(err)
				}
				benchFinalizeInputSink = out
			}
		})
	}
}

// TestInputHeader covers the read half of FinalizeInputHeaders directly. It
// walks the JSON with jsonparser rather than unmarshalling it, so every shape
// encoding/json used to handle has to be handled explicitly here.
func TestInputHeader(t *testing.T) {
	t.Run("missing header key yields an empty header", func(t *testing.T) {
		header, err := inputHeader([]byte(`{"method":"GET"}`))
		require.NoError(t, err)
		assert.Empty(t, header)
	})

	t.Run("null header yields an empty header", func(t *testing.T) {
		header, err := inputHeader([]byte(`{"header":null}`))
		require.NoError(t, err)
		assert.Empty(t, header)
	})

	t.Run("non-object header is an error", func(t *testing.T) {
		_, err := inputHeader([]byte(`{"header":[1,2,3]}`))
		assert.Error(t, err)
	})

	t.Run("array and scalar values are both accepted", func(t *testing.T) {
		header, err := inputHeader([]byte(`{"header":{"X-List":["a","b"],"X-Scalar":"c"}}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, header.Values("X-List"))
		assert.Equal(t, []string{"c"}, header.Values("X-Scalar"))
	})

	t.Run("keys are canonicalized and merged", func(t *testing.T) {
		header, err := inputHeader([]byte(`{"header":{"authorization":"first","AUTHORIZATION":["second"]}}`))
		require.NoError(t, err)
		assert.Len(t, header, 1, "differently cased keys must collapse into one entry")
		assert.ElementsMatch(t, []string{"first", "second"}, header.Values("Authorization"))
	})

	t.Run("escaped values are unescaped", func(t *testing.T) {
		header, err := inputHeader([]byte(`{"header":{"X-Escaped":["a\"b\\c\nd&e"]}}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"a\"b\\c\nd&e"}, header.Values("X-Escaped"))
	})

	t.Run("null value keeps the key with no values", func(t *testing.T) {
		header, err := inputHeader([]byte(`{"header":{"X-Empty":null}}`))
		require.NoError(t, err)
		values, exists := header["X-Empty"]
		assert.True(t, exists, "the key must survive the round trip")
		assert.Nil(t, values)
	})

	t.Run("non-string values are an error", func(t *testing.T) {
		_, err := inputHeader([]byte(`{"header":{"X-Number":42}}`))
		assert.Error(t, err)

		_, err = inputHeader([]byte(`{"header":{"X-Number":[42]}}`))
		assert.Error(t, err, "a non-string list entry must be reported, not silently dropped")

		_, err = inputHeader([]byte(`{"header":{"X-Object":{"nested":"value"}}}`))
		assert.Error(t, err)
	})
}

// TestAppendJSONString pins appendJSONString against the encoder it replaces:
// encoding/json with HTML escaping disabled. The escaping rules are subtle
// enough (short forms, \u00xx for the remaining control bytes, U+2028/U+2029,
// invalid UTF-8) that a table of expectations would only restate the
// implementation - comparing against the standard library is the real check.
func TestAppendJSONString(t *testing.T) {
	for _, value := range []string{
		"",
		"plain",
		`with "quotes"`,
		`with \backslash`,
		"tab\tnewline\ncarriage\rreturn",
		"control\x00\x01\x1f",
		"delete\x7f",
		"html & <tags> 'quoted'",
		"unicode: äöü 日本語 🎉",
		"line\u2028separator\u2029paragraph",
		"invalid utf8: \xff\xfe",
		"Bearer eyJhbGciOiJIUzI1NiJ9.e30.abc-_123",
	} {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			var expected bytes.Buffer
			encoder := json.NewEncoder(&expected)
			encoder.SetEscapeHTML(false)
			require.NoError(t, encoder.Encode(value))

			assert.Equal(t,
				strings.TrimSuffix(expected.String(), "\n"),
				string(appendJSONString(nil, value)))
		})
	}
}

// TestAppendHeaderJSON pins the object appendJSONString's values end up in -
// most importantly its key order, which subscription connection keys are
// derived from and which therefore has to be stable across requests.
func TestAppendHeaderJSON(t *testing.T) {
	t.Run("empty header", func(t *testing.T) {
		assert.Equal(t, `{}`, string(appendHeaderJSON(nil, http.Header{})))
	})

	t.Run("keys are sorted", func(t *testing.T) {
		header := http.Header{
			"X-Zulu":        []string{"z"},
			"Authorization": []string{"a"},
			"X-Alpha":       []string{"m"},
		}
		expected := `{"Authorization":["a"],"X-Alpha":["m"],"X-Zulu":["z"]}`
		assert.Equal(t, expected, string(appendHeaderJSON(nil, header)))
		// Map iteration order varies per run; repeat to make an accidental
		// dependency on it show up.
		for i := 0; i < 20; i++ {
			assert.Equal(t, expected, string(appendHeaderJSON(nil, header)))
		}
	})

	t.Run("multiple values and a nil value", func(t *testing.T) {
		header := http.Header{"X-Multi": []string{"a", "b"}, "X-Nil": nil}
		assert.Equal(t, `{"X-Multi":["a","b"],"X-Nil":null}`, string(appendHeaderJSON(nil, header)))
	})

	t.Run("appends to the existing buffer", func(t *testing.T) {
		out := appendHeaderJSON([]byte(`prefix:`), http.Header{"A": []string{"b"}})
		assert.Equal(t, `prefix:{"A":["b"]}`, string(out))
	})
}

// TestFinalizeInputHeadersKeepsAwkwardHeaderNames guards the header object
// being rebuilt as a whole rather than key by key: a "." in a header name is
// legal (RFC 9110 tchar) but is path syntax to sjson, so a per-key set would
// nest it instead of writing it out.
func TestFinalizeInputHeadersKeepsAwkwardHeaderNames(t *testing.T) {
	in := []byte(`{"header":{"X.Dotted":["one"],"X-Star*":["two"]}}`)
	out, err := FinalizeInputHeaders(in, func(header http.Header) {
		header.Set("X-Added", "three")
	}, nil)
	require.NoError(t, err)

	header, err := inputHeader(out)
	require.NoError(t, err)
	assert.Equal(t, []string{"one"}, header.Values("X.Dotted"))
	assert.Equal(t, []string{"two"}, header.Values("X-Star*"))
	assert.Equal(t, []string{"three"}, header.Values("X-Added"))
}

// TestFinalizeInputHeadersValueReachesUpstreamVerbatim pins the one visible
// difference from the encoding/json round trip this used to do: json.Marshal
// escapes "&", "<" and ">" as \u0026 and friends, while Do() reads header
// values back out with jsonparser, which does not unescape them - so those
// values used to reach the upstream mangled.
func TestFinalizeInputHeadersValueReachesUpstreamVerbatim(t *testing.T) {
	const value = `a&b<c>d`

	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("X-Ampersand")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	input := SetInputMethod(nil, []byte("GET"))
	input = SetInputURL(input, []byte(server.URL))
	input, err := FinalizeInputHeaders(input, func(header http.Header) {
		header.Set("X-Ampersand", value)
	}, nil)
	require.NoError(t, err)

	require.NoError(t, Do(http.DefaultClient, context.Background(), input, &bytes.Buffer{}))
	assert.Equal(t, value, <-received)
}

// TestFinalizeInputHeadersResultOutlivesThePooledBuffer guards the scratch
// buffer the header JSON is serialized into: it goes back to the pool as soon
// as sjson has written the new input, so an already-returned result must not
// still be pointing at it.
func TestFinalizeInputHeadersResultOutlivesThePooledBuffer(t *testing.T) {
	in := []byte(`{"header":{"Authorization":["original"]}}`)

	first, err := FinalizeInputHeaders(in, func(header http.Header) {
		header.Set("Authorization", "first")
	}, nil)
	require.NoError(t, err)
	firstCopy := string(first)

	for i := 0; i < 10; i++ {
		_, err := FinalizeInputHeaders(in, func(header http.Header) {
			header.Set("Authorization", "second-with-a-much-longer-value-to-force-the-buffer-to-grow")
			header.Set("X-Padding", strings.Repeat("p", 512))
		}, nil)
		require.NoError(t, err)
	}

	assert.Equal(t, firstCopy, string(first),
		"a finalized input must not alias the pooled serialization buffer")
}

// TestFinalizeInputHeadersConcurrent exercises the shared serialization buffer
// pool from several goroutines at once - the whole point of this call site is
// that it runs per request, concurrently.
func TestFinalizeInputHeadersConcurrent(t *testing.T) {
	in := []byte(`{"header":{"X-Static":["value"]},"body":{"query":"{ hello }"}}`)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := fmt.Sprintf("caller-%d", i)
			expected := fmt.Sprintf(`{"header":{"X-Caller":["caller-%d"],"X-Static":["value"]},"body":{"query":"{ hello }"}}`, i)
			for j := 0; j < 50; j++ {
				out, err := FinalizeInputHeaders(in, func(header http.Header) {
					header.Set("X-Caller", value)
				}, nil)
				assert.NoError(t, err)
				assert.Equal(t, expected, string(out))
			}
		}(i)
	}
	wg.Wait()
}
