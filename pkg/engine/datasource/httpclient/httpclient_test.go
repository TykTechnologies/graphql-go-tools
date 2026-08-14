package httpclient

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/internal/pkg/quotes"
	"github.com/TykTechnologies/graphql-go-tools/pkg/lexer/literal"
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
				w.Header().Set(ContentEncodingHeader, EncodingGzip)
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

		t.Run("br", func(t *testing.T) {
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
}
