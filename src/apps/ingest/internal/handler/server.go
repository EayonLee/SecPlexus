// Package handler 定义 Ingest 的 HTTP 协议边界。
package handler

import (
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// Server 持有 Ingest 处理请求所需的依赖。
// 骨架阶段只有就绪检查；通道与连接器路由在后续批次挂载。
type Server struct {
	IAM *iamclient.Client // 用于验证会话与授权的管理 API 客户端。
}
