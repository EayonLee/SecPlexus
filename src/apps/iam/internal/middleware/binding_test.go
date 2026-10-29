package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	httpmiddleware "github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
)

func TestNativeBindingRejectsMalformedAndOversizedCredentials(t *testing.T) {
	engine := gin.New()
	engine.Use(httpmiddleware.InputBoundary(map[string]int64{"/api/v1/iam/auth/login": 2048}))
	engine.POST("/api/v1/iam/auth/login", func(c *gin.Context) {
		var input model.APILoginInput
		if err := httpx.Bind(c, &input); err != nil {
			if !c.Writer.Written() {
				c.Status(400)
			}
			return
		}
		c.Status(204)
	})
	for _, tc := range []struct {
		name, body, media string
		chunked           bool
		status            int
	}{
		{"valid", `{"username":"member","password":"password"}`, "application/json", false, 204},
		{"unknown", `{"username":"member","password":"password","extra":true}`, "application/json", false, 400},
		{"case", `{"Username":"member","password":"password"}`, "application/json", false, 400},
		{"multiple", `{"username":"member","password":"password"} {}`, "application/json", false, 400},
		{"media", `{"username":"member","password":"password"}`, "text/plain", false, 415},
		{"large", `{"username":"member","password":"password"}` + strings.Repeat(" ", 2048), "application/json", false, 413},
		{"large_chunked", `{"username":"member","password":"password"}` + strings.Repeat(" ", 2048), "application/json", true, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/api/v1/iam/auth/login", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", tc.media)
			request.Header.Set("X-CSRF-Token", "validated-by-csrf-boundary")
			request.Header.Set("Origin", "http://localhost")
			if tc.chunked {
				request.ContentLength = -1
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("want %d, got %d", tc.status, response.Code)
			}
		})
	}
}
