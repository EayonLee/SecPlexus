package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/kafka"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/consumer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/event"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// StoreEvents 启动独立 Event 存储进程。Kafka 和 ClickHouse 连接在此次运行完成后回收。
func StoreEvents(ctx context.Context, cfg config.Config) (outcome error) {
	if err := cfg.ValidateStoreEvents(); err != nil {
		return err
	}
	options := cfg.Events
	storage := cfg.ClickHouse

	logger, closeLog, err := logging.Open(cfg.Logging, "ingest-store-events")
	if err != nil {
		return err
	}
	defer func() { outcome = errors.Join(outcome, closeLog()) }()
	slog.SetDefault(logger)

	store, err := event.NewStore(storage.URL, storage.Database, storage.Username, storage.Password, storage.Timeout)
	if err != nil {
		return err
	}
	defer store.Close()
	projector, err := event.NewProjector()
	if err != nil {
		return err
	}

	security, err := kafka.Options(cfg.Kafka.Security(options.Consumer))
	if err != nil {
		return err
	}
	client, err := kgo.NewClient(append([]kgo.Opt{
		kgo.SeedBrokers(cfg.Kafka.Brokers...),
		kgo.ConsumerGroup(options.ConsumerGroup),
		kgo.ConsumeTopics(options.Topic),
		kgo.ConsumeStartOffset(kgo.NewOffset().AtStart()),
		kgo.ConsumeResetOffset(kgo.NoResetOffset()),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.MaxConcurrentFetches(1),
		kgo.FetchMaxBytes(int32(options.BatchBytes)),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordDeliveryTimeout(storage.Timeout),
	}, security...)...)
	if err != nil {
		return fmt.Errorf("create event consumer: %w", err)
	}
	defer client.CloseAllowingRebalance()

	logging.From(ctx).Info("Starting event storage", "event", "ingest.events.store_starting",
		"topic", options.Topic, "consumer_group", options.ConsumerGroup)
	worker := consumer.Worker{
		Kafka: client, Store: store, Projector: projector, FailureTopic: options.FailureTopic,
		MaxRecords: options.BatchRecords, MaxBytes: options.BatchBytes,
		BatchTimeout: storage.Timeout, CheckInterval: options.CheckInterval,
	}
	return worker.Run(ctx)
}
