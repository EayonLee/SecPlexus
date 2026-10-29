package middleware

import (
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// init 在接收请求前注册模块集合规则，让预览和保存复用同一组边界。
func init() {
	engine := binding.Validator.Engine().(*validator.Validate)
	engine.RegisterAlias("iam_permissions", "max=200,unique")
	engine.RegisterAlias("iam_role_ids", "max=50,unique,dive,uuid")
}
