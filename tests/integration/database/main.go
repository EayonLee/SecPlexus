// Command database 管理全栈测试拥有的临时数据库及认证期限 fixture。
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	cachekey "github.com/EayonLee/SecPlexus/src/packages/go/cache/key"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		// 连接错误可能含私有地址或凭据，命令边界只输出操作阶段。
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("test database operation is required")
	}
	connection, err := pgx.Connect(ctx, os.Getenv("SPX_TEST_POSTGRES_URL"))
	if err != nil {
		return errors.New("cannot connect to test PostgreSQL administrator")
	}
	defer func() { _ = connection.Close(context.Background()) }()

	switch args[0] {
	case "create":
		name := "spx_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err = connection.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			return errors.New("cannot create owned test database")
		}
		fmt.Print(name)
		return nil
	case "expire-auth":
		if len(args) != 3 || !strings.HasPrefix(args[1], "spx_e2e_") || len(args[1]) != 40 {
			return errors.New("invalid owned database fixture")
		}
		config := connection.Config().Copy()
		config.Database = args[1]
		owned, openErr := pgx.ConnectConfig(ctx, config)
		if openErr != nil {
			return errors.New("cannot open owned authentication fixture")
		}
		defer func() { _ = owned.Close(ctx) }()
		updated, updateErr := owned.Exec(ctx, `UPDATE iam.user_sessions SET authenticated_at=clock_timestamp()-interval '31 minutes',reauthenticated_at=NULL WHERE id=$1 AND state='active'`, args[2])
		if updateErr != nil || updated.RowsAffected() == 0 {
			return errors.New("cannot expire owned authentication fixture")
		}
		return nil
	case "drop":
		if len(args) != 2 || !strings.HasPrefix(args[1], "spx_e2e_") || len(args[1]) != 40 {
			return errors.New("invalid owned database name")
		}
		config := connection.Config().Copy()
		config.Database = args[1]
		if err = cleanAccounts(ctx, config); err != nil {
			return err
		}
		if _, err = connection.Exec(ctx, "DROP DATABASE "+pgx.Identifier{args[1]}.Sanitize()+" WITH (FORCE)"); err != nil {
			return errors.New("cannot remove owned test database")
		}
		return nil
	default:
		return errors.New("unsupported test database operation")
	}
}

// cleanAccounts 仅清理临时库唯一账号/会话的桶，共享来源桶保持原 TTL。
func cleanAccounts(ctx context.Context, config *pgx.ConnConfig) error {
	owned, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return errors.New("cannot open owned test database for cleanup")
	}
	defer func() { _ = owned.Close(context.Background()) }()
	var schema bool
	if err = owned.QueryRow(ctx, "SELECT to_regclass('iam.users') IS NOT NULL").Scan(&schema); err != nil {
		return errors.New("cannot inspect owned schema")
	}
	if !schema {
		return nil
	}
	rows, err := owned.Query(ctx, "SELECT username_normalized FROM iam.users UNION ALL SELECT 'password:'||id::text FROM iam.user_sessions")
	if err != nil {
		return errors.New("cannot inspect owned accounts")
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var account string
		if err = rows.Scan(&account); err != nil {
			return errors.New("cannot read owned account")
		}
		digest := sha256.Sum256([]byte(account))
		keys = append(keys, cachekey.IAMLoginAccount.Format(hex.EncodeToString(digest[:])))
	}
	if rows.Err() != nil {
		return errors.New("cannot read owned accounts")
	}
	if len(keys) == 0 {
		return nil
	}
	options, err := redis.ParseURL(os.Getenv("SPX_TEST_REDIS_URL"))
	if err != nil {
		return errors.New("invalid test Redis URL")
	}
	client := redis.NewClient(options)
	defer func() { _ = client.Close() }()
	if err = client.Del(ctx, keys...).Err(); err != nil {
		return errors.New("owned Redis key cleanup failed")
	}
	return nil
}
