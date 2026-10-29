package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// TestErrorMappingKeepsHTTPAndPublicCode 校验真实状态语义，未知错误和数据库 Detail 不能返回前端。
func TestErrorMappingKeepsHTTPAndPublicCode(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   result.Code
	}{
		{repository.ErrUnauthenticated, 401, result.CodeUnauthenticated},
		{authentication.ErrInvalidCredentials, 401, result.CodeInvalidCredentials},
		{repository.ErrInvalid, 400, result.CodeInvalidRequest},
		{context.DeadlineExceeded, 503, result.CodeServiceUnavailable},
		{context.Canceled, 503, result.CodeServiceUnavailable},
		{&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("secret-connection")}, 503, result.CodeServiceUnavailable},
		{&pgconn.PgError{Code: "23505", Detail: "secret-row"}, 409, result.CodeConflict},
		{&pgconn.PgError{Code: "57P01", Detail: "secret-shutdown"}, 503, result.CodeServiceUnavailable},
		{errors.New("secret-internal"), 500, result.CodeInternalError},
	}
	for _, item := range cases {
		response := httptest.NewRecorder()
		(&Server{}).writeError(response, httptest.NewRequest("POST", "/", nil), item.err)
		var result result.Result[any]
		if json.Unmarshal(response.Body.Bytes(), &result) != nil || response.Code != item.status || result.Code != item.code || result.Data != nil || result.RequestID == "" {
			t.Fatal("HTTP status and Result differ")
		}
		if strings.Contains(response.Body.String(), "secret-") {
			t.Fatal("internal error leaked to frontend")
		}
	}
}
