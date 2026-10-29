package router

import (
	"encoding/json"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router/internalapi"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

func TestRoutesMatchContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		source   []byte
		register func(*gin.Engine)
	}{
		{"public", contract.OpenAPI, func(engine *gin.Engine) { RegisterPublic(engine, nil, func(*gin.Context) {}) }},
		{"internal", contract.InternalOpenAPI, func(engine *gin.Engine) { internalapi.RegisterHandlers(engine, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := gin.New()
			tc.register(mux)
			var document struct {
				Paths map[string]map[string]json.RawMessage `json:"paths"`
			}
			if err := json.Unmarshal(tc.source, &document); err != nil {
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
				if strings.HasPrefix(route.Path, "/api/") || strings.HasPrefix(route.Path, "/internal/") {
					path := regexp.MustCompile(`:([a-z_]+)`).ReplaceAllString(route.Path, "{$1}")
					actual = append(actual, route.Method+" "+path)
				}
			}

			sort.Strings(actual)
			sort.Strings(expected)
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("挂载路由与契约不一致：actual=%v expected=%v", actual, expected)
			}
		})
	}
}
