package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aarondl/opt/omit"
	"github.com/gin-gonic/gin"
)

func TestBindingRejectsUnknownTrailingAndInvalidJSON(t *testing.T) {
	for _, body := range []string{`{"name":"ok","unknown":true}`, `{"name":"ok"} {"name":"again"}`, `null`, `{}`} {
		t.Run(body, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/", func(c *gin.Context) {
				var input struct {
					Name string `json:"name" binding:"required"`
				}
				if err := Bind(c, &input); err != nil {
					c.Status(http.StatusBadRequest)
					return
				}
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected rejection, got %d", response.Code)
			}
		})
	}
}

func TestBindingOptionalStringPreservesPresence(t *testing.T) {
	for _, tc := range []struct {
		body           string
		valid, present bool
		value          string
	}{
		{`{}`, true, false, ""}, {`{"description":""}`, true, true, ""}, {`{"description":"value"}`, true, true, "value"}, {`{"description":null}`, false, false, ""},
	} {
		t.Run(tc.body, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var input struct {
				Description omit.Val[string] `json:"description,omitempty" binding:"omitempty,max=1000"`
			}
			err := Bind(c, &input)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
			if tc.valid && (input.Description.IsValue() != tc.present || input.Description.GetOrZero() != tc.value) {
				t.Fatal("presence/value mismatch")
			}
		})
	}
}
