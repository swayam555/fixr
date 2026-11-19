package in.fixr.order.orders;

import in.fixr.order.common.ApiException;
import in.fixr.order.common.Ids;
import jakarta.persistence.*;
import java.security.SecureRandom;
import java.time.Instant;
import java.util.UUID;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

@Entity
@Table(name = "orders")
public class Order {
    private static final SecureRandom RANDOM = new SecureRandom();

    @Id
    private UUID id;
    @Column(name = "customer_id")
    private UUID customerId;
    @Column(name = "pro_id")
    private UUID proId;
    @Column(name = "service_id")
    private String serviceId;
    @Column(name = "zone_id")
    private String zoneId;
    @Column(name = "address_id")
    private UUID addressId;
    @Column(name = "address_summary")
    private String addressSummary;
    private double lat;
    private double lng;
    @Enumerated(EnumType.STRING)
    private OrderStatus status;
    @Version
    private long version;
    @Column(name = "price_paise")
    private int pricePaise;
    private String notes;
    @JdbcTypeCode(SqlTypes.JSON)
    private String scope;
    @Column(name = "start_code")
    private String startCode;
    @Column(name = "eta_sec")
    private Integer etaSec;
    private int redispatches;
    @Column(name = "cancel_reason")
    private String cancelReason;
    @Column(name = "idempotency_key")
    private String idempotencyKey;
    @Column(name = "searching_at")
    private Instant searchingAt;
    @Column(name = "assigned_at")
    private Instant assignedAt;
    @Column(name = "arrived_at")
    private Instant arrivedAt;
    @Column(name = "started_at")
    private Instant startedAt;
    @Column(name = "completed_at")
    private Instant completedAt;
    @Column(name = "closed_at")
    private Instant closedAt;
    @Column(name = "created_at")
    private Instant createdAt;
    @Column(name = "updated_at")
    private Instant updatedAt;

    protected Order() {}

    public Order(UUID customerId, String serviceId, String zoneId, UUID addressId, String addressSummary,
                 double lat, double lng, int pricePaise, String notes, String scope, String idempotencyKey) {
        this.id = Ids.newId();
        this.customerId = customerId;
        this.serviceId = serviceId;
        this.zoneId = zoneId;
        this.addressId = addressId;
        this.addressSummary = addressSummary;
        this.lat = lat;
        this.lng = lng;
        this.pricePaise = pricePaise;
        this.notes = notes;
        this.scope = scope;
        this.idempotencyKey = idempotencyKey;
        this.startCode = String.format("%04d", RANDOM.nextInt(10_000));
        this.status = OrderStatus.SEARCHING;
        this.createdAt = Instant.now();
        this.searchingAt = createdAt;
        this.updatedAt = createdAt;
    }

    /** Moves the order and stamps the matching timestamp; illegal moves are a 409, never a silent no-op. */
    OrderStatus moveTo(OrderStatus to) {
        if (!status.canMoveTo(to)) {
            throw ApiException.conflict("ILLEGAL_TRANSITION", "Order is " + status.name().toLowerCase().replace('_', ' ')
                    + " and cannot become " + to.name().toLowerCase().replace('_', ' '));
        }
        OrderStatus from = status;
        Instant now = Instant.now();
        status = to;
        updatedAt = now;
        switch (to) {
            case SEARCHING -> { searchingAt = now; proId = null; etaSec = null; redispatches++; }
            case ASSIGNED -> assignedAt = now;
            case ARRIVED -> arrivedAt = now;
            case IN_PROGRESS -> startedAt = now;
            case COMPLETED -> { completedAt = now; closedAt = now; }
            case CANCELLED, UNFULFILLED -> closedAt = now;
            default -> { }
        }
        return from;
    }

    void assign(UUID proId, Integer etaSec) {
        this.proId = proId;
        this.etaSec = etaSec;
    }

    void cancelReason(String reason) {
        this.cancelReason = reason;
    }

    public UUID getId() { return id; }
    public UUID getCustomerId() { return customerId; }
    public UUID getProId() { return proId; }
    public String getServiceId() { return serviceId; }
    public String getZoneId() { return zoneId; }
    public UUID getAddressId() { return addressId; }
    public String getAddressSummary() { return addressSummary; }
    public double getLat() { return lat; }
    public double getLng() { return lng; }
    public OrderStatus getStatus() { return status; }
    public long getVersion() { return version; }
    public int getPricePaise() { return pricePaise; }
    public String getNotes() { return notes; }
    public String getScope() { return scope; }
    public String getStartCode() { return startCode; }
    public Integer getEtaSec() { return etaSec; }
    public int getRedispatches() { return redispatches; }
    public String getCancelReason() { return cancelReason; }
    public Instant getSearchingAt() { return searchingAt; }
    public Instant getAssignedAt() { return assignedAt; }
    public Instant getArrivedAt() { return arrivedAt; }
    public Instant getStartedAt() { return startedAt; }
    public Instant getCompletedAt() { return completedAt; }
    public Instant getClosedAt() { return closedAt; }
    public Instant getCreatedAt() { return createdAt; }
}
