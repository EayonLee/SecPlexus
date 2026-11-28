package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/sqlcgen"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
)

// LoadInputs 提供有效的运行配置、停用版本和每个接入的独立错误。
func LoadInputs(ctx context.Context, pool *pgxpool.Pool) (input.Snapshot, error) {
	rows, err := sqlcgen.New(pool).LoadSources(ctx)
	if err != nil {
		return input.Snapshot{}, fmt.Errorf("could not read sources: %w", err)
	}

	snapshot := input.Snapshot{Targets: make(map[string]input.SourceActivation, len(rows))}

	compiled := make(map[[sha256.Size]byte]*normalizer.Compiled)

	for _, row := range rows {
		target := input.SourceActivation{SourceVersion: input.SourceVersion{
			Revision: row.Revision, CredentialGeneration: row.CredentialGeneration,
			ParsingRuleVersion: int(row.ParsingRuleVersion), Enabled: row.Enabled,
		}}
		snapshot.Targets[row.ID] = target
		if !row.Enabled {
			continue
		}
		// 协议配置损坏只隔离本行，避免影响同一快照的其他来源。
		var decoded protocolConfiguration
		if err = json.Unmarshal(row.Configuration, &decoded); err != nil {
			target.Problem = "configuration_invalid"
			snapshot.Targets[row.ID] = target
			continue
		}

		var token []byte
		if decoded.TokenSHA256 != "" {
			if token, err = hex.DecodeString(decoded.TokenSHA256); err != nil || len(token) != sha256.Size {
				target.Problem = "credential_invalid"
				snapshot.Targets[row.ID] = target
				continue
			}
		}

		entry := input.Entry{

			ID: row.ID, Name: row.Name, Vendor: row.Vendor, Product: row.Product,
			Revision: row.Revision, CredentialGeneration: row.CredentialGeneration,
			Protocol: input.Protocol(row.Protocol),
			Kafka:    input.KafkaBinding{Credential: decoded.Credential, Brokers: decoded.Brokers, Topic: decoded.Topic, StartPosition: decoded.StartPosition, CreatedAt: row.CreatedAt},
		}
		if decoded.Port != nil {
			entry.Port = int(*decoded.Port)
		}

		if len(token) == len(entry.Token) {
			copy(entry.Token[:], token)
		}

		if row.Definition != nil {
			digest := sha256.Sum256(row.Definition)
			engine := compiled[digest]
			if engine == nil {
				var compileErr error
				engine, compileErr = normalizer.CompileDefinition(row.Definition)
				if compileErr != nil {
					target.Problem = "rule_invalid"
					snapshot.Targets[row.ID] = target

					continue
				}
				// 只在当前快照复用不可变规则，后续刷新自然释放不再使用的版本。
				compiled[digest] = engine
			}

			entry.Mapping = engine
			entry.ParsingRuleID = *row.ParsingRuleID
			entry.ParsingRuleVersion = int(row.ParsingRuleVersion)
		}

		snapshot.Entries = append(snapshot.Entries, entry)
	}

	return snapshot, nil
}

// ReloadInterval 是接入点刷新间隔；SIGHUP 会立即触发一次。
const ReloadInterval = 5 * time.Second
