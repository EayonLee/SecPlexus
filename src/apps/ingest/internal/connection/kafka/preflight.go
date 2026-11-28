package kafka

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/kversion"
)

// CheckEvents 核对部署准备的 Kafka 资源，不创建业务主题或发送测试事件。
func CheckEvents(ctx context.Context, brokers []string, security Security, eventTopic, failureTopic, consumerGroup string, retentionMS int64) error {
	options, err := Options(security)
	if err != nil {
		return err
	}
	if consumerGroup != "" {
		versions := kversion.Stable()
		// v5 可核验尚未创建的消费组权限，v6 将这种情况作为组不存在处理。
		versions.SetMaxKeyVersion(kmsg.NewPtrDescribeGroupsRequest().Key(), 5)
		options = append(options, kgo.MaxVersions(versions))
	}
	client, err := kgo.NewClient(append(options, kgo.SeedBrokers(brokers...))...)
	if err != nil {
		return err
	}
	defer client.Close()

	const write = 1 << kmsg.ACLOperationWrite
	const read = 1 << kmsg.ACLOperationRead
	authorized := map[string]int32{eventTopic: write}
	if failureTopic != "" {
		authorized[eventTopic] = read
		authorized[failureTopic] = write
	}
	request := kmsg.NewPtrMetadataRequest()
	request.AllowAutoTopicCreation = false
	request.IncludeTopicAuthorizedOperations = true
	for topic := range authorized {
		request.Topics = append(request.Topics, kmsg.MetadataRequestTopic{Topic: &topic})
	}
	metadata, err := readEventTopicMetadata(ctx, client, request)
	if err != nil {
		return fmt.Errorf("could not read Kafka topic metadata: %w", err)
	}

	if len(metadata.Topics) != len(authorized) {
		return errors.New("Kafka did not return metadata for all event topics")
	}

	configs := kmsg.NewPtrDescribeConfigsRequest()
	for _, topic := range metadata.Topics {
		if topic.Topic == nil || topic.ErrorCode != 0 || topic.TopicID == [16]byte{} || len(topic.Partitions) == 0 {
			return errors.New("Kafka topic is unavailable or has no stable topic identity")
		}
		name := *topic.Topic
		if topic.AuthorizedOperations != math.MinInt32 && topic.AuthorizedOperations&authorized[name] != authorized[name] {
			return fmt.Errorf("Kafka topic permission is missing: %s", name)
		}
		for _, partition := range topic.Partitions {
			if partition.ErrorCode != 0 || partition.Leader < 0 {
				return fmt.Errorf("Kafka partition has no leader: %s/%d", name, partition.Partition)
			}
		}
		configs.Resources = append(configs.Resources, kmsg.DescribeConfigsRequestResource{
			ResourceType: kmsg.ConfigResourceTypeTopic, ResourceName: name, ConfigNames: []string{"retention.ms", "cleanup.policy", "retention.bytes"},
		})
	}

	settings, err := configs.RequestWith(ctx, client)
	if err != nil || len(settings.Resources) != len(authorized) {
		return errors.New("could not inspect Kafka topic retention")
	}
	want := topicSettings(retentionMS)
	for _, resource := range settings.Resources {
		if resource.ErrorCode != 0 {
			return fmt.Errorf("Kafka topic settings are unavailable: %s", resource.ResourceName)
		}
		actual := make(map[string]string, len(resource.Configs))
		for _, item := range resource.Configs {
			if item.Value != nil {
				actual[item.Name] = *item.Value
			}
		}
		for _, item := range want {
			if actual[item.Name] != *item.Value {
				return fmt.Errorf("Kafka topic %s requires %s=%s", resource.ResourceName, item.Name, *item.Value)
			}
		}
	}
	if consumerGroup != "" {
		request := kmsg.NewPtrDescribeGroupsRequest()
		request.Groups = []string{consumerGroup}
		request.IncludeAuthorizedOperations = true
		groups, err := request.RequestWith(ctx, client)
		// 首次查询会触发 broker 创建内部位移主题，等待协调器就绪后再判断权限。
		for errors.Is(err, kerr.CoordinatorNotAvailable) || errors.Is(err, kerr.CoordinatorLoadInProgress) {
			select {
			case <-ctx.Done():
				return fmt.Errorf("Kafka event consumer group coordinator is not ready: %w", ctx.Err())
			case <-time.After(250 * time.Millisecond):
				groups, err = request.RequestWith(ctx, client)
			}
		}
		if errors.Is(err, kerr.GroupAuthorizationFailed) {
			return fmt.Errorf("Kafka event consumer group requires READ permission: %s", consumerGroup)
		}
		if err != nil {
			return fmt.Errorf("inspect Kafka event consumer group %s: %w", consumerGroup, err)
		}
		if len(groups.Groups) != 1 || groups.Groups[0].Group != consumerGroup {
			return fmt.Errorf("could not inspect Kafka event consumer group: %s", consumerGroup)
		}
		group := groups.Groups[0]
		if group.ErrorCode != 0 || (group.AuthorizedOperations != math.MinInt32 && group.AuthorizedOperations&read == 0) {
			return fmt.Errorf("Kafka event consumer group requires READ permission: %s", consumerGroup)
		}
	}
	return nil
}

// InitEventTopics 创建部署拥有的两个主题，已有主题由运维显式修改。
func InitEventTopics(ctx context.Context, brokers []string, security Security, events, failures string, retentionMS int64) error {
	if events == "" || failures == "" || events == failures {
		return errors.New("event and failure topics must be distinct and nonempty")
	}
	options, err := Options(security)
	if err != nil {
		return err
	}
	client, err := kgo.NewClient(append(options, kgo.SeedBrokers(brokers...))...)
	if err != nil {
		return err
	}
	defer client.Close()

	request := kmsg.NewPtrCreateTopicsRequest()
	for _, name := range []string{events, failures} {
		request.Topics = append(request.Topics, kmsg.CreateTopicsRequestTopic{
			Topic: name, NumPartitions: -1, ReplicationFactor: -1, Configs: topicSettings(retentionMS),
		})
	}
	response, err := request.RequestWith(ctx, client)
	if err != nil {
		return fmt.Errorf("create Kafka event topics: %w", err)
	}
	if len(response.Topics) != len(request.Topics) {
		return errors.New("Kafka topic initialization returned incomplete results")
	}
	for _, topic := range response.Topics {
		if err := kerr.ErrorForCode(topic.ErrorCode); err != nil && !errors.Is(err, kerr.TopicAlreadyExists) {
			return fmt.Errorf("create Kafka topic %s: %w", topic.Topic, err)
		}
	}
	return nil
}

func topicSettings(retentionMS int64) []kmsg.CreateTopicsRequestTopicConfig {
	retention, cleanup, size := strconv.FormatInt(retentionMS, 10), "delete", "-1"
	return []kmsg.CreateTopicsRequestTopicConfig{
		{Name: "retention.ms", Value: &retention},
		{Name: "cleanup.policy", Value: &cleanup},
		{Name: "retention.bytes", Value: &size},
	}
}

// readEventTopicMetadata 给刚创建的主题留出 leader 可见时间。权限错误由调用方直接处理。
func readEventTopicMetadata(ctx context.Context, client *kgo.Client, request *kmsg.MetadataRequest) (*kmsg.MetadataResponse, error) {
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		metadata, err := request.RequestWith(check, client)
		if err != nil {
			return nil, err
		}
		pending := false
		for _, topic := range metadata.Topics {
			switch kerr.ErrorForCode(topic.ErrorCode) {
			case kerr.UnknownTopicOrPartition, kerr.LeaderNotAvailable:
				pending = true
			case nil:
				pending = pending || topic.TopicID == [16]byte{} || len(topic.Partitions) == 0
				for _, partition := range topic.Partitions {
					pending = pending || partition.Leader < 0 || partition.ErrorCode == kerr.LeaderNotAvailable.Code
				}
			}
		}
		if !pending {
			return metadata, nil
		}
		select {
		case <-check.Done():
			return metadata, nil
		case <-time.After(100 * time.Millisecond):
		}
	}
}
