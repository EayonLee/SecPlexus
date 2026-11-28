// Package kafkafixture 管理集成测试独占的 Kafka 资源。
package kafkafixture

import (
	"context"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// CreateTopic 独占创建主题，同名主题属于其他运行，不能接管。
func CreateTopic(client *kgo.Client, name string, partitions int32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := kmsg.NewPtrCreateTopicsRequest()
	topic := kmsg.NewCreateTopicsRequestTopic()
	topic.Topic, topic.NumPartitions, topic.ReplicationFactor = name, partitions, 1
	req.Topics = append(req.Topics, topic)
	res, err := req.RequestWith(ctx, client)
	if err != nil {
		return err
	}
	if len(res.Topics) != 1 || res.Topics[0].Topic != name {
		return errors.New("broker did not acknowledge the requested topic")
	}
	return kerr.ErrorForCode(res.Topics[0].ErrorCode)
}

// DeleteTopic 删除本次创建的确切主题，允许重复回收。
func DeleteTopic(client *kgo.Client, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := kmsg.NewPtrDeleteTopicsRequest()
	req.TopicNames = []string{name}
	req.Topics = []kmsg.DeleteTopicsRequestTopic{{Topic: &name}}
	res, err := req.RequestWith(ctx, client)
	if err != nil {
		return err
	}
	if len(res.Topics) != 1 {
		return errors.New("missing topic deletion result")
	}
	err = kerr.ErrorForCode(res.Topics[0].ErrorCode)
	if errors.Is(err, kerr.UnknownTopicOrPartition) {
		return nil
	}
	return err
}

// DeleteGroup 仅回收指定的测试消费组，尚未建立的消费组无需处理。
func DeleteGroup(client *kgo.Client, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := kmsg.NewPtrDeleteGroupsRequest()
	req.Groups = []string{name}
	res, err := req.RequestWith(ctx, client)
	if err != nil {
		return err
	}
	if len(res.Groups) != 1 || res.Groups[0].Group != name {
		return errors.New("missing group deletion result")
	}
	err = kerr.ErrorForCode(res.Groups[0].ErrorCode)
	if errors.Is(err, kerr.GroupIDNotFound) {
		return nil
	}
	return err
}
