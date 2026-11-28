// Command kafka-admin 为浏览器测试创建和回收临时 Kafka 资源。
package main

import (
	"context"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/EayonLee/SecPlexus/tests/integration/kafkafixture"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("expected operation and resource name")
	}
	action, name := os.Args[1], os.Args[2]
	client, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(os.Getenv("SPX_TEST_KAFKA_BROKERS"), ",")...))
	if err != nil {
		log.Fatal("invalid test Kafka configuration")
	}
	defer client.Close()

	switch {
	case action == "ready" && name == "broker":
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = client.Ping(ctx)
		cancel()
	case regexp.MustCompile(`^spx-e2e-[0-9a-f]{32}\.(input|ingest\.(events|event-failures))$`).MatchString(name):
		switch action {
		case "create-topic":
			err = kafkafixture.CreateTopic(client, name, 1)
		case "delete-topic":
			err = kafkafixture.DeleteTopic(client, name)
		default:
			log.Fatal("invalid test topic operation")
		}
	case action == "delete-group" && (regexp.MustCompile(`^spx-ingest-[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`).MatchString(name) ||
		regexp.MustCompile(`^spx-e2e-[0-9a-f]{32}-ingest-events$`).MatchString(name)):
		err = kafkafixture.DeleteGroup(client, name)
	default:
		log.Fatal("invalid test resource")
	}
	if err != nil {
		log.Fatalf("%s %s: %v", action, name, err)
	}
}
