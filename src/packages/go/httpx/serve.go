package httpx

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Serve 统一拥有本进程的监听，任一启动失败或上下文取消都会释放全部监听与活跃请求。
// TLS 服务通过 Server.TLSConfig 提供已加载的证书，初始化失败不能留下其他已启动的监听。
func Serve(ctx context.Context, servers ...*http.Server) error {
	listeners := make([]net.Listener, 0, len(servers))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	for _, server := range servers {
		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			return err
		}
		listeners = append(listeners, listener)
	}
	return serveListeners(ctx, servers, listeners)
}

// serveListeners 只管理已经拥有的监听和请求取消，绑定失败的释放归 Serve。
func serveListeners(ctx context.Context, servers []*http.Server, listeners []net.Listener) error {
	group, serving := errgroup.WithContext(ctx)
	for i, server := range servers {
		baseContext := server.BaseContext
		server.BaseContext = func(listener net.Listener) context.Context {
			if baseContext == nil {
				return serving
			}
			// 保留调用方的值和取消语义，同时由进程退出信号取消活跃请求。
			base, cancel := context.WithCancel(baseContext(listener))
			context.AfterFunc(serving, cancel)
			return base
		}
		listener := listeners[i]
		group.Go(func() error {
			var err error
			if server.TLSConfig != nil {
				err = server.ServeTLS(listener, "", "")
			} else {
				err = server.Serve(listener)
			}
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		})
	}
	group.Go(func() error {
		<-serving.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var draining sync.WaitGroup
		outcomes := make([]error, len(servers))
		// 全部监听同时停止接收请求，共享同一个退出时限。
		for i, server := range servers {
			draining.Go(func() {
				if err := server.Shutdown(shutdown); err != nil {
					outcomes[i] = errors.Join(err, server.Close())
				}
			})
		}
		draining.Wait()
		return errors.Join(outcomes...)
	})
	return group.Wait()
}
