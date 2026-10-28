// Package handler 按 IAM 授权范围处理 Console 请求，不信任浏览器声明的身份。
package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/router"
	"github.com/EayonLee/SecPlexus/src/contracts/common"
	consoleapi "github.com/EayonLee/SecPlexus/src/contracts/console"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Server 使用 IAM 的授权结果处理 Console 请求。
type Server struct {
	IAM *iamclient.Client // 用于验证会话及组织权限的 IAM 客户端。
}

// authorize 验证原始会话并取得当前组织范围，拒绝不可信的浏览器身份声明。
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) (contract.CapabilityDecision, bool) {
	org, err := uuid.Parse(chi.URLParam(r, "org_id"))
	if err != nil {
		result.Fail(w, common.CodeInvalidRequest)
		return contract.CapabilityDecision{}, false
	}

	decision, err := s.IAM.Authorize(r.Context(), r, contract.PermissionIAMOrganizationsRead, org.String(), nil)
	if err != nil {
		var failure *iamclient.Error
		if errors.As(err, &failure) {
			if failure.Code == common.CodeRateLimited {
				w.Header().Set("Retry-After", "60")
			}
			result.Fail(w, failure.Code)
		} else {
			result.Fail(w, common.CodeServiceUnavailable)
		}

		return contract.CapabilityDecision{}, false
	}

	if decision.Scope.Kind != "organization" || decision.Scope.OrgID != org.String() {
		logging.From(r.Context()).Error("IAM 返回的组织范围不匹配", "event", "console.scope_rejected", "org_id", org.String())
		result.Fail(w, common.CodeServiceUnavailable)
		return contract.CapabilityDecision{}, false
	}

	return decision, true
}

// Handler 注册 Console 路由并返回 HTTP 处理器。
func (s *Server) Handler() http.Handler {
	mux := chi.NewRouter()
	router.Register(mux, s.routeHandlers())
	return middleware.Boundary(mux, mux)
}

// context 返回已经 IAM 验证的当前组织上下文。
func (s *Server) context(w http.ResponseWriter, r *http.Request) {
	decision, ok := s.authorize(w, r)
	if !ok {
		return
	}

	result.Success(w, consoleapi.APIConsoleContext{
		OrgID:      decision.Scope.OrgID,
		UserID:     decision.Principal.UserID,
		DecisionID: decision.DecisionID,
	})
}

// events 订阅组织事件并定期重新鉴权，权限撤销后结束流。
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	decision, ok := s.authorize(w, r)
	if !ok {
		return
	}

	logger := logging.From(r.Context()).With("org_id", decision.Scope.OrgID, "user_id", decision.Principal.UserID, "decision_id", decision.DecisionID)
	started := time.Now()
	reason := "client_disconnected"
	defer func() {
		logger.Info("组织事件流结束", "event", "sse.closed", "reason", reason, "duration_ms", time.Since(started).Milliseconds())
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		reason = "flush_unsupported"
		logger.Error("响应不支持流式刷新", "event", "sse.unsupported")
		return
	}

	if _, err := fmt.Fprintf(w, "event: authorized\ndata: {\"decision_id\":%q}\n\n", decision.DecisionID); err != nil {
		return
	}

	flusher.Flush()
	logger.Info("组织事件流已建立", "event", "sse.opened")
	// 在三十秒撤权目标内，为有界 IAM 调用预留五秒。
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			next, err := s.IAM.Authorize(r.Context(), r, contract.PermissionIAMOrganizationsRead, decision.Scope.OrgID, nil)
			if err != nil || !next.Allowed || next.Scope.Kind != "organization" || next.Scope.OrgID != decision.Scope.OrgID {
				reason = "access_revoked"
				if err != nil {
					reason = "reauthorization_failed"
				}
				logger.Warn("事件流重新鉴权未通过", "event", "sse.access_stopped", "reason", reason, "error", logging.Error(err))
				_, _ = fmt.Fprint(w, "event: access_revoked\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			if _, err = fmt.Fprint(w, ": authorized\n\n"); err != nil {
				reason = "write_failed"
				logger.Debug("事件流发送失败", "event", "sse.write_failed", "error", logging.Error(err))
				return
			}
			flusher.Flush()
		}
	}
}
