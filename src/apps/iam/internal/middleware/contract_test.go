package middleware

import (
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	httpmiddleware "github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContractBindingRejectsMalformedAndOversizedCredentials(t *testing.T) {
	validate, err := httpmiddleware.ContractValidation(contract.OpenAPI, map[string]int64{"/api/v1/iam/auth/login": 2048})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(validate)
	engine.POST("/api/v1/iam/auth/login", func(c *gin.Context) {
		var input contract.APILoginInput
		if err := httpx.Bind(c, &input); err != nil {
			t.Fatal(err)
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
