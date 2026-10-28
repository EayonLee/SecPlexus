// Package management 处理用户、组织、角色和审计管理接口。
package management

import "github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"

// Handler 显式接收业务用例与共享密码计算预算。
type Handler struct {
	Service   *service.Service
	HashSlots chan struct{}
}
