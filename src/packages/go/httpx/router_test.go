package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRouterErrorsPreserveHTTPMethodSemantics(t *testing.T) {
	router := chi.NewRouter()
	RouterErrors(router)
	router.Get("/resource", func(http.ResponseWriter, *http.Request) {})
	for _, test := range []struct {
		method, path string
		status       int
	}{{"GET", "/missing", 404}, {"POST", "/resource", 405}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.status {
			t.Fatalf("%s %s returned %d", test.method, test.path, w.Code)
		}

		if test.status == 405 && w.Header().Get("Allow") != "GET" {
			t.Fatalf("method response lacks Allow: %v", w.Header())
		}
	}
}
