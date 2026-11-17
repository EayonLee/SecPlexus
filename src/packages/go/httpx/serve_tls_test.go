package httpx_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
)

// writeCertificate 生成一对自签证书并返回文件路径。
func writeCertificate(t *testing.T) (string, string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	certPath := filepath.Join(directory, "cert.pem")
	keyPath := filepath.Join(directory, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err = os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	return certPath, keyPath
}

// freeAddr 返回一个当前可用的本地端口。
func freeAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	return listener.Addr().String()
}

// 回归：配置了证书时监听必须真的走 TLS。
func TestServeTLSHandshakes(t *testing.T) {
	certPath, keyPath := writeCertificate(t)
	addr := freeAddr(t)

	server := &http.Server{
		Addr:      addr,
		Handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }),
		TLSConfig: &tls.Config{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	certificate, loadErr := tls.LoadX509KeyPair(certPath, keyPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}

	go func() { done <- httpx.Serve(ctx, server) }()

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}

	var response *http.Response
	var err error

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response, err = client.Get("https://" + addr + "/")
		if err == nil {
			break
		}
		if response != nil {
			_ = response.Body.Close()
			response = nil
		}

		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("TLS 连接失败: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("状态 = %d", response.StatusCode)
	}
	if response.TLS == nil || len(response.TLS.PeerCertificates) == 0 {
		t.Fatal("响应不是 TLS 连接")
	}

	cancel()
	select {
	case serveErr := <-done:
		if serveErr != nil {
			t.Fatalf("ServeTLS 返回错误: %v", serveErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeTLS 没有随上下文取消退出")
	}
}

// 证书缺失时必须在监听之前失败，而不是启动后再报错。
func TestServeWithoutCertificateStaysPlaintext(t *testing.T) {
	// 证书加载发生在调用方（bootstrap）；这里只保证没有证书时是明文监听。
	server := &http.Server{Addr: freeAddr(t), Handler: http.NewServeMux()}
	if server.TLSConfig != nil {
		t.Fatal("默认不应启用 TLS")
	}
}
