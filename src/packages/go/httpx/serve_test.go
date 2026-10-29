package httpx

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestServeReleasesEarlierListenerWhenLaterBindFails(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := first.Addr().String()
	_ = first.Close()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = occupied.Close() }()
	err = Serve(context.Background(), &http.Server{Addr: address}, &http.Server{Addr: occupied.Addr().String()})
	var failure *net.OpError
	if !errors.As(err, &failure) {
		t.Fatalf("missing listener cause: %v", err)
	}
	available, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal("earlier listener leaked", err)
	}
	_ = available.Close()
}

func TestServeCancelsActiveEventStreamOnShutdown(t *testing.T) {
	for _, mode := range []string{"default", "custom"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = listener.Close() }()
			type contextKey struct{}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, mode))
			defer cancel()
			exited := make(chan struct{})
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Context().Value(contextKey{}) != mode {
					t.Error("request lost its configured base context value")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if _, err := w.Write([]byte("event: ready\ndata: {}\n\n")); err != nil {
					t.Error(err)
					return
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(exited)
			})}
			if mode == "custom" {
				server.BaseContext = func(net.Listener) context.Context {
					return context.WithValue(context.Background(), contextKey{}, mode)
				}
			}
			done := make(chan error, 1)
			go func() { done <- serveListeners(ctx, []*http.Server{server}, []net.Listener{listener}) }()
			t.Cleanup(func() {
				cancel()
				_ = server.Close()
			})
			deadline, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			request, _ := http.NewRequestWithContext(deadline, "GET", "http://"+listener.Addr().String()+"/events", nil)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if line, err := bufio.NewReader(response.Body).ReadString('\n'); err != nil || line != "event: ready\n" {
				t.Fatal(line, err)
			}
			cancel()
			select {
			case <-exited:
			case <-deadline.Done():
				t.Fatal("active stream did not observe cancellation")
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-deadline.Done():
				t.Fatal("shutdown needed forced termination")
			}
			if connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
				_ = connection.Close()
				t.Fatal("listener survived shutdown")
			}
		})
	}
}

func TestServeStopsEveryListenerBeforeWaitingForActiveRequests(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	entered, released := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(released) })
	firstStopping, secondStopping := make(chan struct{}), make(chan struct{})
	servers := []*http.Server{
		{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			// 模拟已经开始、需要完成后才能退出的非取消式工作。
			<-released
			w.WriteHeader(http.StatusNoContent)
		})},
		{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })},
	}
	servers[0].RegisterOnShutdown(func() { close(firstStopping) })
	servers[1].RegisterOnShutdown(func() { close(secondStopping) })
	done := make(chan error, 1)
	go func() { done <- serveListeners(ctx, servers, []net.Listener{first, second}) }()
	t.Cleanup(func() {
		cancel()
		release()
		for _, server := range servers {
			_ = server.Close()
		}
	})
	deadline, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	request, _ := http.NewRequestWithContext(deadline, "GET", "http://"+first.Addr().String(), nil)
	requestDone := make(chan error, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			_ = response.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-entered:
	case <-deadline.Done():
		t.Fatal("owned request did not start")
	}
	cancel()
	for _, stopping := range []<-chan struct{}{firstStopping, secondStopping} {
		select {
		case <-stopping:
		case <-deadline.Done():
			t.Fatal("later listener waited for an earlier active request")
		}
	}
	if connection, err := net.DialTimeout("tcp", second.Addr().String(), time.Second); err == nil {
		_ = connection.Close()
		t.Fatal("later listener still accepts new requests during shutdown")
	}
	release()
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-deadline.Done():
		t.Fatal("owned servers did not shut down")
	}
}
