//go:build integration

package authentication

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

func TestSessionLifecycleAcrossReplicas(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	secret := "isolated integration password"
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failures <- store.Seed(ctx, "admin", secret)
		}()
	}

	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	first, second := NewSessions(store, cfg), NewSessions(store, cfg)
	user, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := first.Manager.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	principal, err := first.Create(loaded, user, uuid.NewString(), "integration", "127.0.0.1", false)
	if err != nil {
		t.Fatal(err)
	}

	token, expiry, err := first.Manager.Commit(loaded)
	if err != nil {
		t.Fatal(err)
	}

	var data []byte
	if err = store.Pool.QueryRow(ctx, `SELECT data FROM iam.scs_sessions WHERE token=$1`, TokenHash(token)).Scan(&data); err != nil {
		t.Fatal(err)
	}

	replicaContext, err := second.Manager.Load(ctx, token)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = second.Resolve(replicaContext); err != nil {
		t.Fatal(err)
	}

	if err = first.Revoke(loaded, principal, principal.SessionID, false, "logout", uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	// 延迟写入可以恢复库内会话数据，但不能恢复已撤销的逻辑身份。
	if _, err = store.Pool.Exec(ctx, `INSERT INTO iam.scs_sessions(token,data,expiry) VALUES($1,$2,$3) ON CONFLICT(token) DO UPDATE SET data=EXCLUDED.data,expiry=EXCLUDED.expiry`, TokenHash(token), data, expiry); err != nil {
		t.Fatal(err)
	}

	if _, err = second.Resolve(replicaContext); err == nil {
		t.Fatal("revoked logical session was resurrected")
	}
	// 即使另一实例的并发请求稍后写回 SCS 数据，账号凭据轮换后旧逻辑会话也必须失效。
	fresh, _ := first.Manager.Load(ctx, "")
	if _, err = first.Create(fresh, user, uuid.NewString(), "integration", "127.0.0.1", false); err != nil {
		t.Fatal(err)
	}

	previousToken, previousExpiry, err := first.Manager.Commit(fresh)
	if err != nil {
		t.Fatal(err)
	}

	var previousData []byte
	if err = store.Pool.QueryRow(ctx, `SELECT data FROM iam.scs_sessions WHERE token=$1`, TokenHash(previousToken)).Scan(&previousData); err != nil {
		t.Fatal(err)
	}

	nextID := uuid.New()
	if _, err = store.Pool.Exec(ctx, `INSERT INTO iam.users(id,username,username_normalized,display_name,status) VALUES($1,'otheruser','otheruser','Other user','active')`, nextID); err != nil {
		t.Fatal(err)
	}

	if _, err = store.Pool.Exec(ctx, `INSERT INTO iam.password_credentials(user_id,password_hash) VALUES($1,$2)`, nextID, user.PasswordHash); err != nil {
		t.Fatal(err)
	}

	nextUser, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "otheruser")
	if err != nil {
		t.Fatal(err)
	}

	rotated, _ := first.Manager.Load(ctx, previousToken)
	if _, err = first.Create(rotated, nextUser, uuid.NewString(), "integration", "127.0.0.1", false); err != nil {
		t.Fatal(err)
	}

	if _, _, err = first.Manager.Commit(rotated); err != nil {
		t.Fatal(err)
	}

	if _, err = store.Pool.Exec(ctx, `INSERT INTO iam.scs_sessions(token,data,expiry) VALUES($1,$2,$3) ON CONFLICT(token) DO UPDATE SET data=EXCLUDED.data,expiry=EXCLUDED.expiry`, TokenHash(previousToken), previousData, previousExpiry); err != nil {
		t.Fatal(err)
	}

	stale, _ := second.Manager.Load(ctx, previousToken)
	if _, err = second.Resolve(stale); err == nil {
		t.Fatal("account rotation left previous identity usable")
	}
	// 凭据版本改变后，旧密码快照不能创建新会话。
	if _, err = store.Pool.Exec(ctx, `UPDATE iam.users SET auth_version=auth_version+1 WHERE id=$1`, user.ID); err != nil {
		t.Fatal(err)
	}

	other, err := first.Manager.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = first.Create(other, user, uuid.NewString(), "integration", "127.0.0.1", false); err == nil {
		t.Fatal("stale password snapshot created a session")
	}
	// 重复初始化不需要初始密码，也不能重置已有凭据。
	if err = store.Seed(ctx, "admin", ""); err != nil {
		t.Fatal(err)
	}

	again, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	if again.PasswordHash != user.PasswordHash {
		t.Fatal("repeat initialization reset password")
	}
}

func TestSessionIdleAndAbsoluteDeadlines(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	secret := "isolated integration password"
	if err := store.Seed(ctx, "admin", secret); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sessions := NewSessions(store, cfg)
	for _, deadline := range []string{"idle", "absolute"} {
		t.Run(deadline, func(t *testing.T) {
			user, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
			if err != nil {
				t.Fatal(err)
			}

			loaded, err := sessions.Manager.Load(ctx, "")
			if err != nil {
				t.Fatal(err)
			}

			p, err := sessions.Create(loaded, user, uuid.NewString(), "integration", "127.0.0.1", false)
			if err != nil {
				t.Fatal(err)
			}

			token, _, err := sessions.Manager.Commit(loaded)
			if err != nil {
				t.Fatal(err)
			}

			if deadline == "idle" {
				_, err = store.Pool.Exec(ctx, `UPDATE iam.user_sessions SET idle_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, p.SessionID)
			} else {
				_, err = store.Pool.Exec(ctx, `UPDATE iam.user_sessions SET authenticated_at=statement_timestamp()-interval '1 hour',absolute_expires_at=statement_timestamp()-interval '1 second',idle_expires_at=statement_timestamp()-interval '1 second' WHERE id=$1`, p.SessionID)
			}

			if err != nil {
				t.Fatal(err)
			}
			// 再次保存 SCS，模拟旧请求延迟写入存储。
			if _, _, err = sessions.Manager.Commit(loaded); err != nil {
				t.Fatal(err)
			}

			next, err := sessions.Manager.Load(ctx, token)
			if err != nil {
				t.Fatal(err)
			}

			if _, err = sessions.Resolve(next); err == nil {
				t.Fatal("expired session accepted")
			}
		})
	}
	// 直接调整有效期，验证续期不能突破最初的绝对期限。
	user, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	loaded, _ := sessions.Manager.Load(ctx, "")
	p, err := sessions.Create(loaded, user, uuid.NewString(), "integration", "127.0.0.1", false)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err = sessions.Manager.Commit(loaded); err != nil {
		t.Fatal(err)
	}

	var before, after time.Time
	if err = store.Pool.QueryRow(ctx, `SELECT absolute_expires_at FROM iam.user_sessions WHERE id=$1`, p.SessionID).Scan(&before); err != nil {
		t.Fatal(err)
	}

	if _, err = sessions.Resolve(loaded); err != nil {
		t.Fatal(err)
	}

	if err = store.Pool.QueryRow(ctx, `SELECT absolute_expires_at FROM iam.user_sessions WHERE id=$1`, p.SessionID).Scan(&after); err != nil {
		t.Fatal(err)
	}

	if !after.Equal(before) {
		t.Fatal("polling extended absolute expiry")
	}
}

func TestConcurrentReauthenticationConsumesSessionOnce(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	secret := "isolated reauthentication password"
	if err := store.Seed(ctx, "admin", secret); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	first, second := NewSessions(store, cfg), NewSessions(store, cfg)
	user, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	initial, _ := first.Manager.Load(ctx, "")
	old, err := first.Create(initial, user, uuid.NewString(), "integration", "127.0.0.1", false)
	if err != nil {
		t.Fatal(err)
	}

	token, _, err := first.Manager.Commit(initial)
	if err != nil {
		t.Fatal(err)
	}

	a, err := first.Manager.Load(ctx, token)
	if err != nil {
		t.Fatal(err)
	}

	b, err := second.Manager.Load(ctx, token)
	if err != nil {
		t.Fatal(err)
	}

	var originalExpiry time.Time
	if err = store.Pool.QueryRow(ctx, `SELECT absolute_expires_at FROM iam.user_sessions WHERE id=$1`, old.SessionID).Scan(&originalExpiry); err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	for _, attempt := range []struct {
		s   *Sessions
		ctx context.Context
	}{{first, a}, {second, b}} {
		go func() {
			p, err := attempt.s.Create(attempt.ctx, user, uuid.NewString(), "integration", "127.0.0.1", true)
			if err == nil {
				if !p.AuthenticatedAt.Equal(old.AuthenticatedAt) || p.ReauthenticatedAt == nil {
					err = fmt.Errorf("rotation lost authentication timestamps")
				}

				var expiry time.Time
				if queryErr := store.Pool.QueryRow(ctx, `SELECT absolute_expires_at FROM iam.user_sessions WHERE id=$1`, p.SessionID).Scan(&expiry); queryErr != nil {
					err = queryErr
				} else if !expiry.Equal(originalExpiry) {
					err = fmt.Errorf("rotation extended absolute deadline")
				}
			}

			results <- err
		}()
	}

	success, denied := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, repository.ErrUnauthenticated) {
			denied++
		} else {
			t.Fatal(err)
		}
	}

	if success != 1 || denied != 1 {
		t.Fatalf("rotation outcomes: success=%d denied=%d", success, denied)
	}
}
