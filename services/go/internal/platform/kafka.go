package platform

import (
	"github.com/twmb/franz-go/pkg/kgo"
)

func KafkaBrokers() []string { return EnvList("KAFKA_BOOTSTRAP", "localhost:29092") }

// NewProducer: idempotent, acks=all, lz4, small linger so batches form under load without hurting latency.
func NewProducer(opts ...kgo.Opt) (*kgo.Client, error) {
	base := []kgo.Opt{
		kgo.SeedBrokers(KafkaBrokers()...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchCompression(kgo.Lz4Compression()),
		kgo.ProducerLinger(5e6),
	}
	return kgo.NewClient(append(base, opts...)...)
}

// NewConsumer joins a group with manual commits; callers commit after their side effects succeed.
func NewConsumer(group string, topics []string, opts ...kgo.Opt) (*kgo.Client, error) {
	base := []kgo.Opt{
		kgo.SeedBrokers(KafkaBrokers()...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.BlockRebalanceOnPoll(),
	}
	return kgo.NewClient(append(base, opts...)...)
}
