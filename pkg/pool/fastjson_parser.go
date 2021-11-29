package pool

import (
	"github.com/valyala/fastjson"
)

var FastJsonParser = fastjson.ParserPool{}
