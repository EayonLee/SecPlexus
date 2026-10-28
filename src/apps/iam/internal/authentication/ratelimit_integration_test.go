//go:build integration

package authentication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	cachekey "github.com/EayonLee/SecPlexus/src/packages/go/cache/key"
)

func TestLimiterAtomicWindowAndFailClosed(t *testing.T) {
	options, err := redis.ParseURL(os.Getenv("SPX_TEST_REDIS_URL"))
	if err != nil {
		t.Fatal(err)
	}
	options.MaxRetries = -1
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	counter, err := NewLimiter(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ip, account := uuid.NewString(), uuid.NewString()
	for _, rule := range []struct {
		value  string
		key    cachekey.Template
		period time.Duration
		limit  int
		check  func(context.Context, string) error
	}{
		{ip, cachekey.IAMLoginIP, time.Minute, 60, counter.CheckIP}, {account, cachekey.IAMLoginAccount, 15 * time.Minute, 20, counter.CheckAccount},
	} {
		hash := sha256.Sum256([]byte(rule.value))
		key := rule.key.Format(hex.EncodeToString(hash[:]))
		t.Cleanup(func() { client.Del(context.Background(), key) })
		var accepted atomic.Int32
		var wg sync.WaitGroup
		for range rule.limit + 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := rule.check(ctx, rule.value)
				if err == nil {
					accepted.Add(1)
				} else if !errors.Is(err, ErrRateLimited) {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if int(accepted.Load()) != rule.limit {
			t.Fatalf("atomic count = %d, want %d", accepted.Load(), rule.limit)
		}
		ttl, err := client.PTTL(ctx, key).Result()
		if err != nil || ttl <= 0 || ttl > rule.period {
			t.Fatalf("missing/bad window TTL: %v %v", ttl, err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := counter.CheckIP(cancelled, ip); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancelled dependency must fail closed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := counter.CheckAccount(ctx, account); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("dependency must fail closed: %v", err)
	}
}
