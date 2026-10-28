package console

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestPublishedOpenAPI(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromFile("openapi.json")
	if err != nil {
		t.Fatal(err)
	}

	if err = document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
}
