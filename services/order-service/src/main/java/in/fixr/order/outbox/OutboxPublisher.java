package in.fixr.order.outbox;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Timer;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.support.SendResult;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;
import org.springframework.transaction.support.TransactionTemplate;

/**
 * Publishes outbox rows in commit order. A cluster-wide advisory lock keeps exactly one replica publishing
 * at a time, which preserves per-order ordering; rows are stamped only after Kafka acks them. A failure
 * stops the batch at that row so nothing behind it overtakes it.
 */
@Component
public class OutboxPublisher {
    private static final Logger log = LoggerFactory.getLogger(OutboxPublisher.class);
    private static final long LOCK_ID = 0x0f1c_0b0cL;

    private record Row(UUID id, String topic, String key, String payload, Timestamp createdAt) {}

    private final JdbcClient jdbc;
    private final TransactionTemplate tx;
    private final KafkaTemplate<String, String> kafka;
    private final PublishFaults faults;
    private final int batchSize;
    private final Counter published;
    private final Counter failed;
    private final Timer lag;

    public OutboxPublisher(JdbcClient jdbc, TransactionTemplate tx, KafkaTemplate<String, String> kafka, PublishFaults faults,
                           MeterRegistry meters, @Value("${fixr.outbox.batch-size:200}") int batchSize) {
        this.jdbc = jdbc;
        this.tx = tx;
        this.kafka = kafka;
        this.faults = faults;
        this.batchSize = batchSize;
        this.published = meters.counter("fixr_outbox_published_total");
        this.failed = meters.counter("fixr_outbox_publish_failures_total");
        this.lag = Timer.builder("fixr_outbox_lag").description("Commit to Kafka ack").publishPercentileHistogram().register(meters);
    }

    @Scheduled(fixedDelayString = "${fixr.outbox.poll-ms:100}")
    public void tick() {
        try {
            while (Boolean.TRUE.equals(tx.execute(s -> publishBatch()))) {
                // a full batch means more is waiting; keep draining without sleeping
            }
        } catch (RuntimeException e) {
            log.warn("outbox tick failed: {}", e.toString());
        }
    }

    /** @return true when the batch was full and more rows may be waiting. */
    Boolean publishBatch() {
        Boolean locked = jdbc.sql("SELECT pg_try_advisory_xact_lock(?)").param(LOCK_ID).query(Boolean.class).single();
        if (!Boolean.TRUE.equals(locked)) return false;

        List<Row> rows = jdbc.sql("""
                        SELECT id, topic, msg_key, payload::text AS payload, created_at FROM outbox
                        WHERE published_at IS NULL ORDER BY created_at, id LIMIT ?""")
                .param(batchSize)
                .query((rs, i) -> new Row(rs.getObject("id", UUID.class), rs.getString("topic"), rs.getString("msg_key"),
                        rs.getString("payload"), rs.getTimestamp("created_at")))
                .list();
        if (rows.isEmpty()) return false;

        List<CompletableFuture<SendResult<String, String>>> sends = new ArrayList<>(rows.size());
        int sent = 0;
        String error = null;
        for (Row r : rows) {
            if (faults.shouldFail()) {
                error = "injected publish failure";
                break;
            }
            sends.add(kafka.send(new ProducerRecord<>(r.topic(), r.key(), r.payload())));
        }
        List<UUID> done = new ArrayList<>();
        for (int i = 0; i < sends.size(); i++) {
            try {
                sends.get(i).get(10, TimeUnit.SECONDS);
                done.add(rows.get(i).id());
                sent++;
            } catch (Exception e) {
                error = e.getClass().getSimpleName() + ": " + e.getMessage();
                break;
            }
        }
        if (!done.isEmpty()) {
            jdbc.sql("UPDATE outbox SET published_at = now(), attempts = attempts + 1 WHERE id IN (:ids)")
                    .param("ids", done).update();
            Instant now = Instant.now();
            for (int i = 0; i < sent; i++) {
                lag.record(java.time.Duration.between(rows.get(i).createdAt().toInstant(), now));
            }
            published.increment(sent);
        }
        if (error != null) {
            Row stuck = rows.get(sent);
            String msg = error.length() > 300 ? error.substring(0, 300) : error;
            jdbc.sql("UPDATE outbox SET attempts = attempts + 1, last_error = ? WHERE id = ?").param(msg).param(stuck.id()).update();
            failed.increment();
            log.warn("outbox publish failed at {}: {}", stuck.id(), msg);
            return false;
        }
        return rows.size() == batchSize;
    }
}
