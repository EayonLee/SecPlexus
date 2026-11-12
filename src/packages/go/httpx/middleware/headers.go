package middleware

import "github.com/gin-gonic/gin"

// 安全响应头的取值依据 OWASP HTTP Headers Cheat Sheet 与 OWASP Secure Headers Project。
// 这里只放与具体部署无关的常量头；HSTS 也在这里下发——浏览器在明文 HTTP 上会忽略它，
// 因此由反向代理终止 TLS 的部署同样能拿到，无需在服务端猜测真实协议。
//
//   - X-Frame-Options / CSP frame-ancestors：管理台不允许被任何页面内嵌，压住点击劫持。
//   - CSP 只写 frame-ancestors/base-uri/object-src，不写 script-src：
//     API 响应没有可执行上下文，写死 script-src 只会制造维护负担。
//   - Referrer-Policy 显式声明 strict-origin-when-cross-origin，不依赖浏览器默认值。
//   - Permissions-Policy 关掉本平台不使用的浏览器特性。
//   - Cross-Origin-* 把响应隔离在本源内，阻止跨源窗口引用与资源加载。
const (
	headerFrameOptions     = "X-Frame-Options"
	headerContentType      = "X-Content-Type-Options"
	headerReferrer         = "Referrer-Policy"
	headerPermissions      = "Permissions-Policy"
	headerCSP              = "Content-Security-Policy"
	headerHSTS             = "Strict-Transport-Security"
	headerOpenerPolicy     = "Cross-Origin-Opener-Policy"
	headerResourcePolicy   = "Cross-Origin-Resource-Policy"
	valueCSPFrameAncestors = "frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
	valuePermissions       = "camera=(), microphone=(), geolocation=(), usb=(), payment=()"
	valueHSTS              = "max-age=31536000"
)

// SecurityHeaders 为每个响应补齐安全头。
// 必须在其它中间件之前挂载：这样 404/405、panic 恢复和鉴权拒绝的响应也带同样一组头，
// 否则攻击者可以用错误响应判断路由是否存在。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		header.Set(headerContentType, "nosniff")
		header.Set(headerFrameOptions, "DENY")
		header.Set(headerCSP, valueCSPFrameAncestors)
		header.Set(headerReferrer, "strict-origin-when-cross-origin")
		header.Set(headerPermissions, valuePermissions)
		header.Set(headerHSTS, valueHSTS)
		header.Set(headerOpenerPolicy, "same-origin")
		header.Set(headerResourcePolicy, "same-origin")
		// X-XSS-Protection 已废弃且可能引入反射型漏洞，OWASP 建议显式关闭。
		header.Set("X-XSS-Protection", "0")
		c.Next()
	}
}
