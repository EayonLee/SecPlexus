package router

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	contract "github.com/EayonLee/SecPlexus/src/contracts/console"
)

func TestRoutesMatchContract(t *testing.T) {
	mux := chi.NewRouter()
	Register(mux, Handlers{})
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(contract.OpenAPI, &document); err != nil {
		t.Fatal(err)
	}

	expected := []string{}
	for path, methods := range document.Paths {
		for method := range methods {
			expected = append(expected, strings.ToUpper(method)+" "+path)
		}
	}

	actual := []string{}
	err := chi.Walk(mux, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/internal/") {
			actual = append(actual, method+" "+path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("挂载路由与契约不一致：actual=%v expected=%v", actual, expected)
	}
}
