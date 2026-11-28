// Command kafka-send 向浏览器用例自有的入站主题发送一条日志。
package main

import (
	"context"
	"flag"
	"github.com/twmb/franz-go/pkg/kgo"
	"log"
	"strings"
	"time"
)

func main() {
	brokers := flag.String("brokers", "", "bootstrap servers")
	topic := flag.String("topic", "", "test input topic")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(*brokers, ",")...), kgo.AllowAutoTopicCreation())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	if err := client.ProduceSync(ctx, &kgo.Record{Topic: *topic, Value: []byte(`{"message":"input acceptance"}`)}).FirstErr(); err != nil {
		log.Fatal(err)
	}
}
