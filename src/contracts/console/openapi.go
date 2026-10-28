package console

import _ "embed"

// OpenAPI 是与生成类型一起发布的接口契约。
//
//go:embed openapi.json
var OpenAPI []byte
