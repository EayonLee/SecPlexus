# 共享实现

这里只放当前实际共用的能力；业务规则、模块私表和单模块实现留在所属应用，不建设万能工具包或依赖容器。

| Go 包 | 职责 |
|---|---|
| [result](go/result/result.go) | 统一成功/失败响应，自动采用登记的 code、message 和 HTTP 状态 |
| [httpx](go/httpx/bind.go) | 有界请求绑定、schema 校验标记和路由协议边界 |
| [httpx/middleware](go/httpx/middleware/request.go) | 请求标识、异常恢复和访问日志，保留流式能力 |
| [configfile](go/configfile/config.go) | YAML 标量环境占位展开，不建立连接 |
| [logging](go/logging/logging.go) | 结构化日志、请求上下文、安全错误诊断及可选轮转文件 |
| [cache/key](go/cache/key/iam.go) | 按模块定义显式 spx 前缀的类型化 key 模板 |
| [iamclient](go/iamclient/client.go) | 通过内部协议验证原始身份与组织范围，不读取 IAM 私表 |

共享包可以依赖公开 contracts，不依赖 apps 的私有实现。配置只负责解析和校验；各模块 bootstrap 持有连接创建和关闭职责。其他语言按公共协议实现各自客户端，不提前建立尚不存在的共享库。
