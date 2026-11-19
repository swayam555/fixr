package in.fixr.order.orders;

import java.util.UUID;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;
import org.springframework.transaction.support.TransactionTemplate;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.json.JsonMapper;

/** Consumes dispatch.results. Dedupe and the state change share one transaction, so redelivery is a no-op. */
@Component
public class DispatchResultListener {
    private static final Logger log = LoggerFactory.getLogger(DispatchResultListener.class);

    private final OrderService orders;
    private final JdbcClient jdbc;
    private final TransactionTemplate tx;
    private final JsonMapper json;

    public DispatchResultListener(OrderService orders, JdbcClient jdbc, TransactionTemplate tx, JsonMapper json) {
        this.orders = orders;
        this.jdbc = jdbc;
        this.tx = tx;
        this.json = json;
    }

    @KafkaListener(topics = "dispatch.results", groupId = "order-service")
    public void onResult(String value) {
        JsonNode ev = json.readTree(value);
        String eventId = ev.path("eventId").asString("");
        String type = ev.path("type").asString("");
        if (eventId.isEmpty() || ev.path("orderId").isMissingNode()) {
            log.warn("dropping malformed dispatch result");
            return;
        }
        UUID orderId = UUID.fromString(ev.path("orderId").asString());
        tx.executeWithoutResult(s -> {
            int fresh = jdbc.sql("INSERT INTO processed_events (consumer, event_id) VALUES ('dispatch-results', ?) ON CONFLICT DO NOTHING")
                    .param(eventId).update();
            if (fresh == 0) return;
            switch (type) {
                case "ACCEPTED" -> {
                    UUID pro = UUID.fromString(ev.path("proId").asString());
                    Integer eta = ev.hasNonNull("etaSec") ? ev.get("etaSec").asInt() : null;
                    if (!orders.applyAccepted(orderId, pro, eta)) {
                        log.info("late accept for {} by {}, released", orderId, pro);
                    }
                }
                case "UNFULFILLED" -> orders.applyUnfulfilled(orderId);
                default -> log.warn("unknown dispatch result type {}", type);
            }
        });
    }
}
