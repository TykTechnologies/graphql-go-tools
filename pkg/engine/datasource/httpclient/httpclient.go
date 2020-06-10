package httpclient

import (
	"bytes"
	"context"
	"io"
	"unicode"

	"github.com/buger/jsonparser"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/quotes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

const (
	PATH        = "path"
	URL         = "url"
	BASEURL     = "base_url"
	METHOD      = "method"
	BODY        = "body"
	HEADERS     = "headers"
	QUERYPARAMS = "query_params"
)

var (
	inputPaths = [][]string{
		{URL},
		{METHOD},
		{BODY},
		{HEADERS},
		{QUERYPARAMS},
	}
)

type Client interface {
	Do(ctx context.Context, requestInput []byte, out io.Writer) (err error)
}

func wrapQuotesIfString(b []byte) []byte {
	inType := gjson.ParseBytes(b).Type
	switch inType {
	case gjson.Number, gjson.String:
		return b
	case gjson.JSON:
		for _,i := range b[1:]{
			if unicode.IsSpace(rune(i)){
				continue
			}
			if i == '"'{
				return b
			}
			break
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

func SetInputHeaders(input, headers []byte) []byte {
	if len(headers) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, HEADERS, wrapQuotesIfString(headers))
	return out
}

func SetInputQueryParams(input, queryParams []byte) []byte {
	if len(queryParams) == 0 {
		return input
	}
	out, _ := sjson.SetRawBytes(input, QUERYPARAMS, wrapQuotesIfString(queryParams))
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
