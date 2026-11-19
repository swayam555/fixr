package in.fixr.order.orders;

import in.fixr.order.address.Address;
import in.fixr.order.address.AddressRepository;
import in.fixr.order.catalogue.CatalogueRepository;
import in.fixr.order.catalogue.ServiceItem;
import in.fixr.order.common.ApiException;
import in.fixr.order.outbox.Outbox;
import in.fixr.order.user.User;
import in.fixr.order.user.UserRepository;
import io.micrometer.core.instrument.MeterRegistry;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class OrderService {
    static final int MAX_OPEN_ORDERS = 3;
    static final int MAX_REDISPATCHES = 2;
    private static final List<OrderStatus> OPEN = List.of(OrderStatus.SEARCHING, OrderStatus.ASSIGNED,
            OrderStatus.EN_ROUTE, OrderStatus.ARRIVED, OrderStatus.IN_PROGRESS);

    private final OrderRepository orders;
    private final TransitionRepository transitions;
    private final AddressRepository addresses;
    private final CatalogueRepository catalogue;
    private final UserRepository users;
    private final Outbox outbox;
    private final MeterRegistry meters;

    public OrderService(OrderRepository orders, TransitionRepository transitions, AddressRepository addresses,
                        CatalogueRepository catalogue, UserRepository users, Outbox outbox, MeterRegistry meters) {
        this.orders = orders;
        this.transitions = transitions;
        this.addresses = addresses;
        this.catalogue = catalogue;
        this.users = users;
        this.outbox = outbox;
        this.meters = meters;
    }

    public record Created(Order order, boolean replayed) {}

    @Transactional
    public Created create(UUID customerId, String idempotencyKey, String serviceId, UUID addressId, String notes) {
        var existing = orders.findByCustomerIdAndIdempotencyKey(customerId, idempotencyKey);
        if (existing.isPresent()) {
            Order o = existing.get();
            if (!o.getServiceId().equals(serviceId) || !o.getAddressId().equals(addressId)) {
                throw ApiException.conflict("IDEMPOTENCY_MISMATCH", "This request key was already used for a different booking");
            }
            return new Created(o, true);
        }
        ServiceItem svc = catalogue.findById(serviceId).filter(ServiceItem::isActive)
                .orElseThrow(() -> ApiException.notFound("Service"));
        Address addr = addresses.findById(addressId)
                .filter(a -> a.getUserId().equals(customerId) && !a.isDeleted())
                .orElseThrow(() -> ApiException.notFound("Address"));
        if (addr.getZoneId() == null) {
            throw ApiException.badRequest("NOT_SERVICEABLE", "We do not serve this address yet");
        }
        if (orders.countByCustomerIdAndStatusIn(customerId, OPEN) >= MAX_OPEN_ORDERS) {
            throw ApiException.conflict("TOO_MANY_OPEN_ORDERS", "Finish or cancel an open booking first");
        }
        Order o = new Order(customerId, svc.getId(), addr.getZoneId(), addr.getId(), addr.summary(), addr.getLat(),
                addr.getLng(), svc.getPricePaise(), notes, null, idempotencyKey);
        orders.saveAndFlush(o);
        transitions.save(new Transition(o.getId(), null, OrderStatus.SEARCHING, Actor.CUSTOMER, customerId, null));
        Map<String, Object> payload = payload(o, svc, null);
        outbox.orderEvent(o.getId(), "ORDER_CREATED", payload);
        outbox.orderEvent(o.getId(), "ORDER_SEARCHING", payload);
        meters.counter("fixr_orders_created_total", "service", svc.getId(), "zone", o.getZoneId()).increment();
        return new Created(o, false);
    }

    @Transactional
    public Order cancelByCustomer(UUID orderId, UUID customerId, Long expectedVersion, String reason) {
        Order o = load(orderId);
        if (!o.getCustomerId().equals(customerId)) throw ApiException.notFound("Order");
        checkVersion(o, expectedVersion);
        UUID pro = o.getProId();
        o.cancelReason(reason);
        return transition(o, OrderStatus.CANCELLED, Actor.CUSTOMER, customerId, reason, pro);
    }

    public enum ProAction { EN_ROUTE, ARRIVED, START, COMPLETE, DROP }

    @Transactional
    public Order proAction(UUID orderId, UUID proId, ProAction action, Long expectedVersion, String code, String reason) {
        Order o = load(orderId);
        if (o.getProId() == null || !o.getProId().equals(proId)) throw ApiException.notFound("Order");
        checkVersion(o, expectedVersion);
        return switch (action) {
            case EN_ROUTE -> transition(o, OrderStatus.EN_ROUTE, Actor.PRO, proId, null, proId);
            case ARRIVED -> transition(o, OrderStatus.ARRIVED, Actor.PRO, proId, null, proId);
            case START -> {
                if (code == null || !java.security.MessageDigest.isEqual(code.getBytes(), o.getStartCode().getBytes())) {
                    throw ApiException.badRequest("WRONG_START_CODE", "That code does not match. Ask the customer to read it again.");
                }
                yield transition(o, OrderStatus.IN_PROGRESS, Actor.PRO, proId, null, proId);
            }
            case COMPLETE -> transition(o, OrderStatus.COMPLETED, Actor.PRO, proId, null, proId);
            case DROP -> {
                if (o.getRedispatches() >= MAX_REDISPATCHES) {
                    o.cancelReason("No pro could complete the booking");
                    yield transition(o, OrderStatus.UNFULFILLED, Actor.PRO, proId, reason, proId);
                }
                yield transition(o, OrderStatus.SEARCHING, Actor.PRO, proId, reason, proId);
            }
        };
    }

    /** Applies a dispatch result. Returns false when the order had already moved on (the pro must be freed). */
    @Transactional
    public boolean applyAccepted(UUID orderId, UUID proId, Integer etaSec) {
        Order o = load(orderId);
        if (o.getStatus() != OrderStatus.SEARCHING) {
            if (proId.equals(o.getProId())) return true;
            outbox.orderEvent(o.getId(), "ORDER_ASSIGN_REJECTED", Map.of("proId", proId.toString(), "zoneId", o.getZoneId(),
                    "status", o.getStatus().name(), "version", o.getVersion()));
            return false;
        }
        o.assign(proId, etaSec);
        transition(o, OrderStatus.ASSIGNED, Actor.DISPATCH, null, null, proId);
        return true;
    }

    @Transactional
    public void applyUnfulfilled(UUID orderId) {
        Order o = load(orderId);
        if (o.getStatus() != OrderStatus.SEARCHING) return;
        o.cancelReason("No pro available nearby");
        transition(o, OrderStatus.UNFULFILLED, Actor.DISPATCH, null, null, null);
    }

    public Order load(UUID id) {
        return orders.findById(id).orElseThrow(() -> ApiException.notFound("Order"));
    }

    private Order transition(Order o, OrderStatus to, Actor actor, UUID actorId, String note, UUID eventProId) {
        OrderStatus from = o.moveTo(to);
        orders.saveAndFlush(o);
        transitions.save(new Transition(o.getId(), from, to, actor, actorId, note));
        ServiceItem svc = catalogue.findById(o.getServiceId()).orElseThrow();
        String type = to == OrderStatus.SEARCHING ? "ORDER_REDISPATCH" : "ORDER_" + to.name();
        outbox.orderEvent(o.getId(), type, payload(o, svc, eventProId));
        meters.counter("fixr_order_transitions_total", "to", to.name(), "actor", actor.name()).increment();
        return o;
    }

    private void checkVersion(Order o, Long expected) {
        if (expected != null && expected != o.getVersion()) {
            throw ApiException.conflict("STALE_ORDER", "The order changed while you were updating it. Refresh and retry.");
        }
    }

    /** The envelope payload every downstream service reads; see services/go/internal/dispatch/types.go. */
    private Map<String, Object> payload(Order o, ServiceItem svc, UUID proId) {
        Map<String, Object> p = new LinkedHashMap<>();
        p.put("customerId", o.getCustomerId().toString());
        UUID pro = proId != null ? proId : o.getProId();
        if (pro != null) {
            p.put("proId", pro.toString());
            p.put("proName", users.findById(pro).map(User::getName).orElse(null));
        }
        p.put("zoneId", o.getZoneId());
        p.put("status", o.getStatus().name());
        p.put("version", o.getVersion());
        p.put("serviceName", svc.getName());
        p.put("skill", svc.getSkill());
        p.put("skillLevel", svc.getSkillLevel());
        p.put("priceBand", svc.priceBand());
        p.put("pricePaise", o.getPricePaise());
        p.put("tools", svc.getTools());
        p.put("estMinutes", svc.getEstMinutes());
        p.put("lat", o.getLat());
        p.put("lng", o.getLng());
        p.put("addressSummary", o.getAddressSummary());
        p.put("createdMs", o.getCreatedAt().toEpochMilli());
        p.put("searchingMs", o.getSearchingAt().toEpochMilli());
        if (o.getEtaSec() != null) p.put("etaSec", o.getEtaSec());
        return p;
    }
}
