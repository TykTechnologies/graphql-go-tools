package httpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"unicode/utf8"

	"github.com/buger/jsonparser"
	bytetemplate "github.com/jensneuse/byte-template"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/TykTechnologies/graphql-go-tools/internal/pkg/quotes"
	"github.com/TykTechnologies/graphql-go-tools/pkg/lexer/literal"
)

const (
	PATH                = "path"
	URL                 = "url"
	URLENCODEBODY       = "url_encode_body"
	BASEURL             = "base_url"
	METHOD              = "method"
	BODY                = "body"
	HEADER              = "header"
	QUERYPARAMS         = "query_params"
	USESSE              = "use_sse"
	SSEMETHODPOST       = "sse_method_post"
	SCHEME              = "scheme"
	HOST                = "host"
	UNNULLVARIABLES     = "unnull_variables"
	UNDEFINED_VARIABLES = "undefined"
)

var (
	inputPaths = [][]string{
		{URL},
		{METHOD},
		{BODY},
		{HEADER},
		{QUERYPARAMS},
	}
	subscriptionInputPaths = [][]string{
		{URL},
		{HEADER},
		{BODY},
	}
)

func wrapQuotesIfString(b []byte) []byte {

	if bytes.HasPrefix(b, []byte("$$")) && bytes.HasSuffix(b, []byte("$$")) {
		return b
	}

	if bytes.HasPrefix(b, []byte("{{")) && bytes.HasSuffix(b, []byte("}}")) {
		return b
	}

	inType := gjson.ParseBytes(b).Type
	switch inType {
	case gjson.Number, gjson.String:
		return b
	case gjson.JSON:
		var value interface{}
		withoutTemplate := bytes.ReplaceAll(b, []byte("$$"), nil)

		buf := &bytes.Buffer{}
		tmpl := bytetemplate.New()
		_, _ = tmpl.Execute(buf, withoutTemplate, func(w io.Writer, path []byte) (n int, err error) {
			return w.Write([]byte("0"))
		})

		withoutTemplate = buf.Bytes()

		err := json.Unmarshal(withoutTemplate, &value)
		if err == nil {
			return b
		}
	case gjson.False:
		if bytes.Equal(b, literal.FALSE) {
			return b
		}
	case gjson.True:
		if bytes.Equal(b, literal.TRUE) {
			return b
		}
	case gjson.Null:
		if bytes.Equal(b, literal.NULL) {
			return b
		}
	}
	return quotes.WrapBytes(b)
}

func SetInputURL(input, url []byte) []byte {
	if len(url) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, URL, wrapQuotesIfString(url))
	return out
}

func SetInputURLEncodeBody(input []byte, urlEncodeBody bool) []byte {
	if !urlEncodeBody {
		return input
	}
	out, _ := sjson.SetRawBytes(input, URLENCODEBODY, []byte("true"))
	return out
}

func SetInputFlag(input []byte, flagName string) []byte {
	out, _ := sjson.SetRawBytes(input, flagName, []byte("true"))
	return out
}

func IsInputFlagSet(input []byte, flagName string) bool {
	value, dataType, _, err := jsonparser.Get(input, flagName)
	if err != nil {
		return false
	}
	if dataType != jsonparser.Boolean {
		return false
	}
	return bytes.Equal(value, literal.TRUE)
}

func SetInputMethod(input, method []byte) []byte {
	if len(method) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, METHOD, wrapQuotesIfString(method))
	return out
}

func SetInputBody(input, body []byte) []byte {
	return SetInputBodyWithPath(input, body, "")
}

func SetInputBodyWithPath(input, body []byte, path string) []byte {
	if len(body) == 0 {
		return input
	}
	if path != "" {
		path = BODY + "." + path
	} else {
		path = BODY
	}
	out, _ := sjson.SetRawBytes(input, path, wrapQuotesIfString(body))
	return out
}

func SetInputHeader(input, headers []byte) []byte {
	if len(headers) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, HEADER, wrapQuotesIfString(headers))
	return out
}

func ApplyHeaderModifier(input []byte, modifier func(http.Header)) []byte {
	modified, err := FinalizeInputHeaders(input, modifier, nil)
	if err != nil {
		return input
	}
	return modified
}

func MergeInputHeader(input []byte, upstreamHeaders http.Header) []byte {
	modified, err := FinalizeInputHeaders(input, nil, upstreamHeaders)
	if err != nil {
		return input
	}
	return modified
}

func FinalizeInputHeaders(input []byte, modifier func(http.Header), upstreamHeaders http.Header) ([]byte, error) {
	if modifier == nil && len(upstreamHeaders) == 0 {
		return input, nil
	}

	header, err := inputHeader(input)
	if err != nil {
		return nil, err
	}
	if modifier != nil {
		modifier(header)
	}
	for key, values := range upstreamHeaders {
		header[http.CanonicalHeaderKey(key)] = append([]string(nil), values...)
	}

	buf := headerBufferPool.Get().(*[]byte)
	defer headerBufferPool.Put(buf)
	*buf = appendHeaderJSON((*buf)[:0], header)

	modified, err := sjson.SetRawBytes(input, HEADER, *buf)
	if err != nil {
		return nil, fmt.Errorf("set datasource input header: %w", err)
	}
	return modified, nil
}

// inputHeader reads the "header" object out of a datasource input. It runs once
// per fetch, so it walks the object with jsonparser rather than unmarshalling
// it: encoding/json would allocate an intermediate map plus a json.RawMessage
// and a []string per entry, all of which are thrown away again here.
func inputHeader(input []byte) (http.Header, error) {
	headerBytes, dataType, _, err := jsonparser.Get(input, HEADER)
	if err != nil || dataType == jsonparser.Null {
		return make(http.Header), nil
	}
	if dataType != jsonparser.Object {
		return nil, fmt.Errorf("datasource input header must be an object, got %s", dataType)
	}

	header := make(http.Header)
	err = jsonparser.ObjectEach(headerBytes, func(key, value []byte, valueType jsonparser.ValueType, _ int) error {
		// ObjectEach hands back unescaped keys but raw values, so only the
		// values need unescaping.
		canonicalKey := http.CanonicalHeaderKey(string(key))
		switch valueType {
		case jsonparser.String:
			// A scalar value is a realistic shape for hand-built or third-party
			// datasource configs; treat it as a single-valued header.
			unescaped, err := jsonparser.Unescape(value, nil)
			if err != nil {
				return fmt.Errorf("unescape datasource input header %q: %w", key, err)
			}
			header[canonicalKey] = append(header[canonicalKey], string(unescaped))
		case jsonparser.Null:
			// encoding/json decoded a null value into an empty list of values.
			// Keep the key present with none so the round trip is unchanged.
			if _, exists := header[canonicalKey]; !exists {
				header[canonicalKey] = nil
			}
		case jsonparser.Array:
			var valueErr error
			if _, err := jsonparser.ArrayEach(value, func(item []byte, itemType jsonparser.ValueType, _ int, err error) {
				if valueErr != nil {
					return
				}
				if err != nil {
					valueErr = err
					return
				}
				if itemType != jsonparser.String {
					valueErr = fmt.Errorf("datasource input header %q must hold strings, got %s", key, itemType)
					return
				}
				unescaped, err := jsonparser.Unescape(item, nil)
				if err != nil {
					valueErr = err
					return
				}
				header[canonicalKey] = append(header[canonicalKey], string(unescaped))
			}); err != nil {
				return fmt.Errorf("read datasource input header %q: %w", key, err)
			}
			if valueErr != nil {
				return fmt.Errorf("read datasource input header %q: %w", key, valueErr)
			}
		default:
			return fmt.Errorf("datasource input header %q must be a string or a list of strings, got %s", key, valueType)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return header, nil
}

// headerBufferPool holds the scratch buffers appendHeaderJSON serializes into.
// The bytes are handed to sjson, which copies them into the returned input, so
// a buffer can go straight back into the pool.
var headerBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 0, 512)
		return &buf
	},
}

// appendHeaderJSON writes header as a JSON object. It replaces
// json.Marshal(http.Header), which spends a reflection walk and a dozen
// allocations on a shape that is known statically here.
func appendHeaderJSON(dst []byte, header http.Header) []byte {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	// encoding/json emits map keys in sorted order, and the serialized input is
	// not just passed along: it is what a subscription's connection and trigger
	// keys are derived from. An unstable key order would stop equivalent
	// subscriptions from sharing an upstream connection.
	sort.Strings(keys)

	dst = append(dst, '{')
	for i, key := range keys {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendJSONString(dst, key)
		dst = append(dst, ':')
		values := header[key]
		if values == nil {
			dst = append(dst, 'n', 'u', 'l', 'l')
			continue
		}
		dst = append(dst, '[')
		for j, value := range values {
			if j > 0 {
				dst = append(dst, ',')
			}
			dst = appendJSONString(dst, value)
		}
		dst = append(dst, ']')
	}
	return append(dst, '}')
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a JSON string literal, matching encoding/json
// with HTML escaping disabled. HTML escaping is deliberately left off: Do()
// reads header values back out with jsonparser, which does not unescape them,
// so a "&" would reach the upstream verbatim.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '"', '\\':
				dst = append(dst, '\\', b)
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xf])
			}
			i++
			start = i
			continue
		}
		char, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case char == utf8.RuneError && size == 1:
			dst = append(dst, s[start:i]...)
			dst = append(dst, "\\ufffd"...)
		case char == '\u2028' || char == '\u2029':
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[char&0xf])
		default:
			i += size
			continue
		}
		i += size
		start = i
	}
	return append(append(dst, s[start:]...), '"')
}

func SetInputQueryParams(input, queryParams []byte) []byte {
	if len(queryParams) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, QUERYPARAMS, wrapQuotesIfString(queryParams))
	return out
}

func SetInputScheme(input, scheme []byte) []byte {
	if len(scheme) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, SCHEME, wrapQuotesIfString(scheme))
	return out
}

func SetInputHost(input, host []byte) []byte {
	if len(host) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, HOST, wrapQuotesIfString(host))
	return out
}

func SetInputPath(input, path []byte) []byte {
	if len(path) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, PATH, wrapQuotesIfString(path))
	return out
}

func requestInputParams(input []byte) (url, method, body, headers, queryParams []byte) {
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			url = bytes
		case 1:
			method = bytes
		case 2:
			body = bytes
		case 3:
			headers = bytes
		case 4:
			queryParams = bytes
		}
	}, inputPaths...)
	return
}

func GetSubscriptionInput(input []byte) (url, header, body []byte) {
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			url = bytes
		case 1:
			header = bytes
		case 2:
			body = bytes
		}
	}, subscriptionInputPaths...)
	return
}

func setUndefinedVariables(data []byte, undefinedVariables []string) ([]byte, error) {
	if len(undefinedVariables) > 0 {
		encoded, err := json.Marshal(undefinedVariables)
		if err != nil {
			return nil, err
		}
		return sjson.SetRawBytes(data, UNDEFINED_VARIABLES, encoded)
	}
	return data, nil
}

func SetUndefinedVariables(data []byte, undefinedVariables []string) []byte {
	result, err := setUndefinedVariables(data, undefinedVariables)
	if err != nil {
		panic(fmt.Errorf("couldn't set undefined variables: %w", err))
	}
	return result
}

func UndefinedVariables(data []byte) []string {
	var undefinedVariables []string
	gjson.GetBytes(data, UNDEFINED_VARIABLES).ForEach(func(key, value gjson.Result) bool {
		undefinedVariables = append(undefinedVariables, value.Str)
		return true
	})
	return undefinedVariables
}
