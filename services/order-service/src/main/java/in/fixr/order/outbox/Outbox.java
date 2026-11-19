package in.fixr.order.outbox;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Propagation;
import org.springframework.transaction.annotation.Transactional;
import tools.jackson.databind.json.JsonMapper;

/** Appends an event to the outbox inside the caller's transaction, so state and event commit together. */
@Component
public class Outbox {
    public static final String ORDER_EVENTS = "order.events";

    private final OutboxRepository repo;
    private final JsonMapper json;

    public Outbox(OutboxRepository repo, JsonMapper json) {
        this.repo = repo;
        this.json = json;
    }

    @Transactional(propagation = Propagation.MANDATORY)
    public UUID orderEvent(UUID orderId, String type, Map<String, Object> payload) {
        UUID eventId = OutboxEvent.newId();
        Map<String, Object> envelope = new LinkedHashMap<>();
        envelope.put("eventId", eventId.toString());
        envelope.put("type", type);
        envelope.put("orderId", orderId.toString());
        envelope.put("atMs", System.currentTimeMillis());
        envelope.put("payload", payload);
        repo.save(new OutboxEvent(eventId, ORDER_EVENTS, orderId.toString(), type, json.writeValueAsString(envelope)));
        return eventId;
    }
}
