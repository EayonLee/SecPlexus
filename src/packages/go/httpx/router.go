package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// RouterErrors 为路由未找到和方法不匹配提供统一 Result。
func RouterErrors(router chi.Router) {
	router.NotFound(func(w http.ResponseWriter, r *http.Request) { result.Fail(w, common.CodeNotFound) })
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		methods := []string{}
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
			if router.Match(chi.NewRouteContext(), method, r.URL.Path) {
				methods = append(methods, method)
			}
		}

		for _, method := range methods {
			w.Header().Add("Allow", method)
		}

		result.Fail(w, common.CodeMethodNotAllowed)
	})
}
