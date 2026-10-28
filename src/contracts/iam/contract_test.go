package iam

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestPublishedOpenAPI(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromData(OpenAPI)
	if err != nil {
		t.Fatal(err)
	}

	if err = document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestVersionsAreDecimalStrings(t *testing.T) {
	data, err := json.Marshal(Principal{AuthVersion: 9007199254740993})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"auth_version":"9007199254740993"`) {
		t.Fatal("version lost wire precision")
	}
}
