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
	timeout := flag.Duration("timeout", 30*time.Second, "upper bound for reading one message")
	flag.Parse()

	if *brokers == "" || *topic == "" {
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

	for {
		fetches := client.PollFetches(ctx)
		if err = fetches.Err(); err != nil {
			fail("poll failed: " + err.Error())
		}

		var payload []byte
		fetches.EachRecord(func(record *kgo.Record) { payload = record.Value })

		if len(payload) > 0 {
			if !json.Valid(payload) {
				fail("event is not valid JSON")
			}

			_, _ = os.Stdout.Write(payload)
			return
		}

		if ctx.Err() != nil {
			fail("no message arrived before the timeout")
		}
	}
}

// fail 只输出阶段，不回显地址或凭据。
func fail(reason string) {
	fmt.Fprintln(os.Stderr, reason)
	os.Exit(1)
}
