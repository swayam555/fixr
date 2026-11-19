package in.fixr.order.outbox;

import in.fixr.order.common.Ids;
import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

@Entity
@Table(name = "outbox")
public class OutboxEvent {
    @Id
    private UUID id;
    private String topic;
    @Column(name = "msg_key")
    private String key;
    @Column(name = "event_type")
    private String type;
    @JdbcTypeCode(SqlTypes.JSON)
    private String payload;

    protected OutboxEvent() {}

    OutboxEvent(UUID id, String topic, String key, String type, String payload) {
        this.id = id;
        this.topic = topic;
        this.key = key;
        this.type = type;
        this.payload = payload;
    }

    public UUID getId() { return id; }
    public String getTopic() { return topic; }
    public String getKey() { return key; }
    public String getType() { return type; }
    public String getPayload() { return payload; }

    static UUID newId() { return Ids.newId(); }
}
