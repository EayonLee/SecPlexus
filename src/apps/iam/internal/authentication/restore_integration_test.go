//go:build integration

package authentication

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

func TestBackupRestoreRevokesRestoredCredentialsBeforeServing(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	password := "isolated restore password"
	if err := store.Seed(ctx, "admin", password); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	sessions := NewSessions(store, cfg)
	user, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := sessions.Manager.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	principal, err := sessions.Create(loaded, user, uuid.NewString(), "integration", "127.0.0.1", false)
	if err != nil {
		t.Fatal(err)
	}

	token, _, err := sessions.Manager.Commit(loaded)
	if err != nil {
		t.Fatal(err)
	}

	connection := store.Pool.Config().ConnConfig
	run := func(tool string, input []byte, args ...string) []byte {
		t.Helper()
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		var command *exec.Cmd
		if container := os.Getenv("SPX_TEST_POSTGRES_CONTAINER"); container != "" {
			command = exec.CommandContext(bounded, "docker", append([]string{"exec", "-i", container, tool, "-U", "postgres", "-d", connection.Database}, args...)...)
		} else {
			command = exec.CommandContext(bounded, tool, append([]string{"-d", connection.Database}, args...)...)
			command.Env = append(os.Environ(), "PGHOST="+connection.Host, "PGPORT="+strconv.Itoa(int(connection.Port)), "PGUSER="+connection.User, "PGPASSWORD="+connection.Password)
		}

		command.Stdin = bytes.NewReader(input)
		data, err := command.Output()
		if err != nil {
			t.Fatalf("isolated %s failed; PostgreSQL client tools are required", tool)
		}

		return data
	}
	backup := run("pg_dump", nil, "--format=custom", "--no-owner")
	if err = sessions.Revoke(loaded, principal, principal.SessionID, false, "logout", uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if _, err = sessions.Resolve(loaded); err == nil {
		t.Fatal("logout did not revoke session")
	}

	run("pg_restore", backup, "--clean", "--if-exists", "--no-owner", "--exit-on-error")
	clear(backup)
	// 数据恢复确实带回了旧会话；维护步骤必须撤销这些恢复出来的身份。
	restored, err := sessions.Manager.Load(ctx, token)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = sessions.Resolve(restored); err != nil {
		t.Fatal("backup did not restore the expected session fixture")
	}

	if err = store.RevokeAfterRestore(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err = sessions.Resolve(restored); err == nil {
		t.Fatal("restored session survived the restore gate")
	}
}
