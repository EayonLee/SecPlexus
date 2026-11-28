package observation

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
)

// Activation 是根据所有尚有受理资格的实例得出的配置生效状态。
type Activation struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// Participate 先登记可能受理流量的实例；未确认快照之前视为仍在应用。
func (s *Store) Participate(ctx context.Context, expires time.Time) error {
	members := s.prefix + "activation:members"
	_, err := s.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRemRangeByScore(ctx, members, "-inf", strconv.FormatInt(time.Now().UnixMilli(), 10))
		pipe.ZAdd(ctx, members, redis.Z{Score: float64(expires.UnixMilli()), Member: s.instance})
		pipe.Expire(ctx, members, lastTTL)
		return nil
	})
	return err
}

// Acknowledge 原子地替换节点的已应用配置，停用确认也带数据库版本。
func (s *Store) Acknowledge(ctx context.Context, targets map[string]input.SourceActivation, expires time.Time) error {
	key := s.prefix + "activation:" + s.instance
	rows := make(map[string]any, len(targets)+1)
	rows["_present"] = "1"
	for id, target := range targets {
		encoded, err := json.Marshal(target)
		if err != nil {
			return err
		}
		rows[id] = encoded
	}
	_, err := s.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, key)
		pipe.HSet(ctx, key, rows)
		pipe.PExpireAt(ctx, key, expires)
		return nil
	})
	return err
}

// Activations 读取当前管理页面中的接入。任意仍能受理流量的节点未确认最新修订时，不能报告已生效。
func (s *Store) Activations(ctx context.Context, ids []string, desired map[string]input.SourceVersion) map[string]Activation {
	query, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	nodes, err := s.activeInstances(query)
	if err != nil || len(nodes) == 0 {
		return nil
	}
	calls := make([]*redis.SliceCmd, 0, len(nodes))
	_, err = s.redis.Pipelined(query, func(pipe redis.Pipeliner) error {
		for _, node := range nodes {
			calls = append(calls, pipe.HMGet(query, s.prefix+"activation:"+node, ids...))
		}
		return nil
	})
	if err != nil {
		return nil
	}

	states := make(map[string]Activation, len(ids))
	for idx, id := range ids {
		wanted := desired[id]
		result := Activation{State: "applied"}
		online := false
		for _, call := range calls {
			raw, valid := call.Val()[idx].(string)
			var applied input.SourceActivation
			if !valid || json.Unmarshal([]byte(raw), &applied) != nil || applied.SourceVersion != wanted || applied.Draining {
				result = Activation{State: "applying"}
				continue
			}
			if applied.Problem != "" {
				result = Activation{State: "failed", Reason: applied.Problem}
				break
			}
			online = online || applied.Online
		}
		if result.State == "applied" && wanted.Enabled && !online {
			result = Activation{State: "applying"}
		}
		states[id] = result
	}
	return states
}

func (s *Store) activeInstances(ctx context.Context) ([]string, error) {
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	return s.redis.ZRangeByScore(ctx, s.prefix+"activation:members", &redis.ZRangeBy{Min: "(" + now, Max: "+inf"}).Result()
}
