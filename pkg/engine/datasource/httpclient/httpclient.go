package httpclient

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/buger/jsonparser"
	byte_template "github.com/jensneuse/byte-template"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/quotes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

const (
	PATH                    = "path"
	URL                     = "url"
	BASEURL                 = "base_url"
	METHOD                  = "method"
	BODY                    = "body"
	HEADER                  = "header"
	QUERYPARAMS             = "query_params"
	SHOWSTATUS              = "show_status"
	SHOWRESPONSEINEXTENSION = "show_response_in_extension"

	SCHEME = "scheme"
	HOST   = "host"
)

var IntFlagBytes = []byte("1")

const (
	RequestInputUrlKeyIdx                     = 0
	RequestInputMethodKeyIdx                  = 1
	RequestInputBodyKeyIdx                    = 2
	RequestInputHeadersKeyIdx                 = 3
	RequestInputQueryParamsKeyIdx             = 4
	RequestInputShowStatusIdx                 = 5
	RequestInputShowResponseAsExtensionKeyIdx = 6
)

var (
	inputPaths = [][]string{
		{URL},
		{METHOD},
		{BODY},
		{HEADER},
		{QUERYPARAMS},
		{SHOWSTATUS},
		{SHOWRESPONSEINEXTENSION},
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
		tmpl := byte_template.New()
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

func SetInputShowStatus(input []byte) []byte {
	out, _ := sjson.SetRawBytes(input, SHOWSTATUS, IntFlagBytes)
	return out
}

func SetInputShowResponseInExtensions(input []byte) []byte {
	out, _ := sjson.SetRawBytes(input, SHOWRESPONSEINEXTENSION, IntFlagBytes)
	return out
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

func RequestInputParams(input []byte) (url, method, body, headers, queryParams []byte, showStatus, showResponseInExtension bool) {
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case RequestInputUrlKeyIdx:
			url = bytes
		case RequestInputMethodKeyIdx:
			method = bytes
		case RequestInputBodyKeyIdx:
			body = bytes
		case RequestInputHeadersKeyIdx:
			headers = bytes
		case RequestInputQueryParamsKeyIdx:
			queryParams = bytes
		case RequestInputShowStatusIdx:
			showStatus = true
		case RequestInputShowResponseAsExtensionKeyIdx:
			showResponseInExtension = true
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
