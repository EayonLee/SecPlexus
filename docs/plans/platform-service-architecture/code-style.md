# 可读性验收示例

本文件说明项目期望的排版与阅读顺序，不是可直接复制的完整生产实现。工具负责机械格式；错误处理、依赖归属和业务约束仍要审查。

## 规则

1. 函数/方法之间空一行，文档注释紧贴声明；生成代码不手改。
2. 函数内部按一段完整工作分块留白：初始化、组装、启动、等待、关闭；不每一行空一行，也不连续堆积无关步骤。
3. 多字段配置与较长调用分行；参数、条件和 SQL 的语义清楚。不能为了短行引入只使用一次且没有独立意义的 helper，也不能把所有装配放进一个巨型函数。
4. 变量使用业务含义；短作用域的 ctx/err/id 保留惯例。不要为了换名改变协议字段或 ORM 列名。
5. 导出符号、关键字段和重要内部函数用中文注释。注释解释职责、单位、失败和并发约束，不能机械重复赋值语句。

## 启动流程示意

以下展示分段方式，具体构造器和资源关闭接口由试点确定，不作为新增通用启动框架的要求。

```go
// serve 组装 Console 依赖，运行 HTTP 服务并在收到终止信号后关闭。
func serve(cfg config.Config) error {
    logger, closeLog, err := logging.Open(cfg.Logging, cfg.App.Name)
    if err != nil {
        return err
    }
    defer closeLog()

    client, err := iamclient.New(cfg.IAM.BaseURL, cfg.IAM.Token, cfg.IAM.CAFile)
    if err != nil {
        return err
    }

    server := &http.Server{
        Addr:              cfg.App.Addr(),
        Handler:           buildHandler(client),
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       15 * time.Second,
        IdleTimeout:       60 * time.Second,
        MaxHeaderBytes:    16 * 1024,
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    logger.Info("正在启动 Console HTTP 服务", "addr", cfg.App.Addr())

    failures := make(chan error, 1)
    go func() {
        failures <- server.ListenAndServe()
    }()

    select {
    case <-ctx.Done():
    case err = <-failures:
    }

    shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    shutdownErr := server.Shutdown(shutdown)
    if errors.Is(err, http.ErrServerClosed) {
        err = nil
    }
    return errors.Join(err, shutdownErr)
}
```

示例省略生产就绪、日志、HTTP handler 构建和客户端资源释放等实现细节；实施必须依照 Spec 的完整生命周期约定。这里的 `buildHandler` 是说明性名称，不要求再造一个只转发的 wrapper。源码需再执行 gofmt，不能把 Markdown 中的缩进当作 Go 格式规范。
