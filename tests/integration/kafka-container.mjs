import { createServer } from "node:net";

// Kafka 必须向客户端通告宿主机映射的端口，因此先选择回环端口，再启动测试 broker。
export async function kafkaContainer(name) {
  const listener = createServer();
  const port = await new Promise((resolve, reject) => {
    listener.once("error", reject);
    listener.listen(0, "127.0.0.1", () => {
      const { port } = listener.address();
      listener.close((error) => error ? reject(error) : resolve(port));
    });
  });
  return {
    brokers: `127.0.0.1:${port}`,
    args: [
      "run", "-d", "--name", name,
      "-e", "KAFKA_NODE_ID=1",
      "-e", "KAFKA_PROCESS_ROLES=broker,controller",
      "-e", "KAFKA_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093",
      "-e", `KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://127.0.0.1:${port}`,
      "-e", "KAFKA_CONTROLLER_QUORUM_VOTERS=1@127.0.0.1:9093",
      "-e", "KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER",
      "-e", "KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT",
      "-e", "KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1",
      "-e", "KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR=1",
      "-e", "KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0",
      "--health-cmd", 'bash -c "exec 3<>/dev/tcp/127.0.0.1/9092"',
      "--health-interval", "3s",
      "--health-retries", "20",
      "-p", `127.0.0.1:${port}:9092`,
      "apache/kafka:4.2.2@sha256:1213eb3943d551e5ed1fca7a4e109001cee35770b66a02a0c37a8964efe09b69",
    ]
  };
}
