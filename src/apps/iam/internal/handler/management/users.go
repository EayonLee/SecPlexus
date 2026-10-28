package management

import (
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// listUsers 按查询条件与分页读取平台用户。
func (s *Handler) listUsers(w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query().Get("q")
	if len(query) > 400 || utf8.RuneCountInString(query) > 100 {
		return repository.ErrInvalid
	}

	limit, offset, err := pagination(r)
	if err != nil {
		return err
	}

	response, err := s.Service.ListUsers(r.Context(), limit, offset, query)
	if err != nil {
		return err
	}

	result.Success(w, response)
	return nil
}

// getUser 读取平台用户的公开字段。
func (s *Handler) getUser(w http.ResponseWriter, r *http.Request) error {
	data, err := s.Service.User(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}

	result.Success(w, json.RawMessage(data))
	return nil
}

// createUser 限制密码计算并创建用户，凭据不会进入结果。
func (s *Handler) createUser(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.Service.Authorize(r.Context(), contract.PermissionIAMPlatformUsersManage, ""); err != nil {
		return err
	}

	var input service.UserInput
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	select {
	case s.HashSlots <- struct{}{}:
		defer func() { <-s.HashSlots }()
	default:
		return authentication.ErrRateLimited
	}

	id, err := s.Service.CreateUser(r.Context(), input, middleware.RequestID(r))
	if err != nil {
		return err
	}

	result.Created(w, map[string]string{"id": id})
	return nil
}

// updateUser 按身份版本更新用户资料和状态。
func (s *Handler) updateUser(w http.ResponseWriter, r *http.Request) error {
	var input service.UserUpdate
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Service.UpdateUser(r.Context(), r.PathValue("id"), input, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}

// setSuperAdmin 处理需要近期认证的管理员身份变更。
func (s *Handler) setSuperAdmin(w http.ResponseWriter, r *http.Request) error {
	var input struct {
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Service.SetSuperAdmin(r.Context(), r.PathValue("id"), input.Enabled, input.Reason, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}

// resetUserPassword 限制密码计算并通过用例重置密码。
func (s *Handler) resetUserPassword(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.Service.Authorize(r.Context(), contract.PermissionIAMPlatformUsersManage, ""); err != nil {
		return err
	}

	var input struct {
		Password string `json:"password"`
	}
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	select {
	case s.HashSlots <- struct{}{}:
		defer func() { <-s.HashSlots }()
	default:
		return authentication.ErrRateLimited
	}

	if err := s.Service.ResetPassword(r.Context(), r.PathValue("id"), input.Password, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}
