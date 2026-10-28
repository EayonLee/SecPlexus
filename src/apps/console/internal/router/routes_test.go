package router

import (
	"encoding/json"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	contract "github.com/EayonLee/SecPlexus/src/contracts/console"
)

func TestRoutesMatchContract(t *testing.T) {
	mux := gin.New()
	Register(mux, nil, func(*gin.Context) {})
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
	for _, route := range mux.Routes() {
		if strings.HasPrefix(route.Path, "/api/") {
			path := regexp.MustCompile(`:([a-z_]+)`).ReplaceAllString(route.Path, "{$1}")
			actual = append(actual, route.Method+" "+path)
		}
	}

	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("挂载路由与契约不一致：actual=%v expected=%v", actual, expected)
	}
}
