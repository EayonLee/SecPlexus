// Command kafka-read 从指定 topic 读取一条消息并输出其 JSON，供端到端用例校验事件形状。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {
	brokers := flag.String("brokers", "", "comma separated broker list")
	topic := flag.String("topic", "", "topic to read from")
	input := flag.String("input", "", "optional source identifier")
	count := flag.Int("count", 1, "number of matching events")
	timeout := flag.Duration("timeout", 30*time.Second, "upper bound for reading one message")
	flag.Parse()

	if *brokers == "" || *topic == "" || *count < 1 || *count > 100 {
		fail("brokers and topic are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(*brokers, ",")...),
		kgo.ConsumeTopics(*topic),
		// 用例要读的是"刚刚推上去的那条"，必须从最早开始，否则会等到超时。
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		fail("could not create client")
	}

	defer client.Close()

	events := make([]json.RawMessage, 0, *count)
	for len(events) < *count {
		fetches := client.PollFetches(ctx)
		if err = fetches.Err(); err != nil {
			fail("poll failed: " + err.Error())
		}
		fetches.EachRecord(func(record *kgo.Record) {
			if len(events) >= *count {
				return
			}
			if !json.Valid(record.Value) {
				fail("event is not valid JSON")
			}
			if *input != "" {
				var event struct {
					Metadata struct {
						SPX struct {
							InputID string `json:"input_id"`
						}
					}
				}
				if err := json.Unmarshal(record.Value, &event); err != nil {
					fail("event is not a JSON object")
				}
				if event.Metadata.SPX.InputID != *input {
					return
				}
			}
			events = append(events, record.Value)
		})
	}
	if *count == 1 {
		_, _ = os.Stdout.Write(events[0])
		return
	}
	if err := json.NewEncoder(os.Stdout).Encode(events); err != nil {
		fail("could not encode events")
	}

}

// fail 只输出阶段，不回显地址或凭据。
func fail(reason string) {
	fmt.Fprintln(os.Stderr, reason)
	os.Exit(1)
}
