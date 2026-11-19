package in.fixr.order.orders;

import in.fixr.order.common.Ids;
import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "order_transitions")
public class Transition {
    @Id
    private UUID id;
    @Column(name = "order_id")
    private UUID orderId;
    @Enumerated(EnumType.STRING)
    @Column(name = "from_status")
    private OrderStatus from;
    @Enumerated(EnumType.STRING)
    @Column(name = "to_status")
    private OrderStatus to;
    @Enumerated(EnumType.STRING)
    private Actor actor;
    @Column(name = "actor_id")
    private UUID actorId;
    private String note;
    @Column(name = "created_at")
    private Instant createdAt;

    protected Transition() {}

    Transition(UUID orderId, OrderStatus from, OrderStatus to, Actor actor, UUID actorId, String note) {
        this.id = Ids.newId();
        this.orderId = orderId;
        this.from = from;
        this.to = to;
        this.actor = actor;
        this.actorId = actorId;
        this.note = note;
        this.createdAt = Instant.now();
    }

    public OrderStatus getFrom() { return from; }
    public OrderStatus getTo() { return to; }
    public Actor getActor() { return actor; }
    public String getNote() { return note; }
    public Instant getCreatedAt() { return createdAt; }
}
