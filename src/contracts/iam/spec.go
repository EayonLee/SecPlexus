package iam

import _ "embed"

//go:embed openapi.json
var OpenAPI []byte

//go:embed internal.json
var InternalOpenAPI []byte
