// Package observation 汇总各 Ingest 实例的短窗口运行统计。
package observation

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

const (
	bucketSize    = time.Minute
	windowBuckets = 5
	bucketTTL     = (windowBuckets + 1) * bucketSize
	presenceTTL   = 15 * time.Second
	lastTTL       = 24 * time.Hour
	keyPrefix     = "spx:ingest:observation:"
)

// Counts 是单个采样窗口的接收、Kafka 确认和解析质量统计。
type Counts struct {
	Received      uint64
	Confirmed     uint64
	Parsed        uint64
	Partial       uint64
	Unclassified  uint64
	LastReceived  int64
	LastConfirmed int64
}

// TrafficStatus 根据 Kafka 确认时间确定当前数据状态。
func TrafficStatus(ready, enabled bool, lastConfirmed int64, now, createdAt time.Time) string {
	switch {
	case !enabled:
		return "disabled"
	case !ready:
		return "unknown"
	case lastConfirmed == 0 && now.Sub(createdAt) >= lastTTL:
		return "unknown"
	case lastConfirmed == 0:
		return "waiting"
	case now.Sub(time.UnixMilli(lastConfirmed)) >= windowBuckets*bucketSize:
		return "quiet"
	default:
		return "receiving"
	}
}

type bucket struct {
	inputID string
	minute  int64
}

// Store 只在发布循环中批量访问 Redis。受理路径更新当前分钟的进程内计数。
type Store struct {
	redis    *redis.Client
	prefix   string
	instance string
	mu       sync.Mutex
	buckets  map[bucket]Counts
}

// New 创建单次进程纪元，跨重启不会覆盖其他实例的计数。
func New(client *redis.Client) *Store {
	return &Store{redis: client, prefix: keyPrefix, instance: uuid.NewString(), buckets: make(map[bucket]Counts)}
}

func (s *Store) count(id string, at time.Time, change func(*Counts)) {
	if id == "" {
		return
	}
	key := bucket{id, at.Truncate(bucketSize).Unix()}
	s.mu.Lock()
	value := s.buckets[key]
	change(&value)
	s.buckets[key] = value
	s.mu.Unlock()
}

// Receive 记录已由管道受理的日志，不依赖解析结果或 broker 回复。
func (s *Store) Receive(id string, at time.Time) {
	s.count(id, at, func(value *Counts) {
		value.Received++
		value.LastReceived = max(value.LastReceived, at.UnixMilli())
	})
}

// Normalize 记录一次归一化的结果。
func (s *Store) Normalize(id string, at time.Time, status pipeline.Status) {
	s.count(id, at, func(value *Counts) {
		switch status {
		case pipeline.StatusOK:
			value.Parsed++
		case pipeline.StatusPartial:
			value.Partial++
		default:
			value.Unclassified++
		}
	})
}

// Confirm 记录出口 broker 已经确认的事件。
func (s *Store) Confirm(id string, at time.Time) {
	s.count(id, at, func(value *Counts) {
		value.Confirmed++
		value.LastConfirmed = max(value.LastConfirmed, at.UnixMilli())
	})
}

var advanceLastReceived = redis.NewScript(`
for i = 1, 2 do
    local stored = tonumber(redis.call('HGET', KEYS[1], tostring(i)) or '0')
    local observed = tonumber(ARGV[i])
    if observed > stored then redis.call('HSET', KEYS[1], tostring(i), ARGV[i]) end
end
redis.call('EXPIRE', KEYS[1], ARGV[3])
return 1
`)

// Flush 将同一实例、分钟的累积值写入固定键。部分失败和重试不重复增加。
func (s *Store) Flush(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	data := make(map[bucket]Counts, len(s.buckets))
	cutoff := now.Truncate(bucketSize).Add(-windowBuckets * bucketSize).Unix()
	for key, value := range s.buckets {
		if key.minute <= cutoff {
			delete(s.buckets, key)
			continue
		}
		data[key] = value
	}
	s.mu.Unlock()

	check, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := s.redis.Pipelined(check, func(pipe redis.Pipeliner) error {
		for key, value := range data {
			encoded, err := json.Marshal(value)
			if err != nil {
				return err
			}
			name := fmt.Sprintf("%s%s:%d", s.prefix, key.inputID, key.minute)
			pipe.HSet(check, name, s.instance, encoded)
			pipe.Expire(check, name, bucketTTL)
			advanceLastReceived.Eval(check, pipe, []string{s.prefix + key.inputID + ":last"},
				value.LastReceived, value.LastConfirmed, int(lastTTL.Seconds()))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.redis.Set(check, s.prefix+"fresh:"+s.instance, "1", presenceTTL).Err()
}

// Run 周期发布，退出时留一个有界窗口送出最后的统计。
func (s *Store) Run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		if err := s.Flush(ctx, time.Now()); err != nil && ctx.Err() == nil {
			logging.From(ctx).Warn("Input observations could not be saved", "event", "input.observation.unavailable",
				"error", logging.Error(err))
		}
		select {
		case <-ctx.Done():
			_ = s.Flush(context.WithoutCancel(ctx), time.Now())
			return
		case <-ticker.C:
		}
	}
}

// Read 返回最近几个分钟桶和最晚接收/确认时间。无新鲜上报时由调用方表示不可确认。
func (s *Store) Read(ctx context.Context, ids []string, now time.Time) (map[string]Counts, bool) {
	query, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	nodes, err := s.activeInstances(query)
	if err != nil || len(nodes) == 0 {
		return nil, false
	}
	keys := make([]string, len(nodes))
	for index, node := range nodes {
		keys[index] = s.prefix + "fresh:" + node
	}
	if count, err := s.redis.Exists(query, keys...).Result(); err != nil || count != int64(len(nodes)) {
		return nil, false
	}
	buckets := make(map[string][]*redis.StringSliceCmd, len(ids))
	last := make(map[string]*redis.SliceCmd, len(ids))
	_, err = s.redis.Pipelined(query, func(pipe redis.Pipeliner) error {
		for _, id := range ids {
			base := s.prefix + id
			last[id] = pipe.HMGet(query, base+":last", "1", "2")
			for i := range windowBuckets {
				minute := now.Truncate(bucketSize).Add(-time.Duration(i) * bucketSize).Unix()
				buckets[id] = append(buckets[id], pipe.HVals(query, base+":"+strconv.FormatInt(minute, 10)))
			}
		}
		return nil
	})
	if err != nil {
		return nil, false
	}
	results := make(map[string]Counts, len(ids))
	for id, commands := range buckets {
		var combined Counts
		for _, command := range commands {
			for _, raw := range command.Val() {
				var value Counts
				if json.Unmarshal([]byte(raw), &value) != nil {
					return nil, false
				}
				combined.Received += value.Received
				combined.Confirmed += value.Confirmed
				combined.Parsed += value.Parsed
				combined.Partial += value.Partial
				combined.Unclassified += value.Unclassified
			}
		}
		timestamps := last[id].Val()
		if len(timestamps) == 2 {
			if text, ok := timestamps[0].(string); ok {
				combined.LastReceived, _ = strconv.ParseInt(text, 10, 64)
			}
			if text, ok := timestamps[1].(string); ok {
				combined.LastConfirmed, _ = strconv.ParseInt(text, 10, 64)
			}
		}
		results[id] = combined
	}
	return results, true
}
