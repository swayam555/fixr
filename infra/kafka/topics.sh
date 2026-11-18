#!/bin/bash
set -euo pipefail
B=kafka:9092
for t in order.events pro.locations dispatch.offers dispatch.results payments.events notifications.out ml.features; do
  /opt/kafka/bin/kafka-topics.sh --bootstrap-server $B --create --if-not-exists --topic "$t" --partitions 6 --replication-factor 1
done
/opt/kafka/bin/kafka-configs.sh --bootstrap-server $B --alter --entity-type topics --entity-name pro.locations --add-config retention.ms=21600000
echo "topics ready"
