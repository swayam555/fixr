package in.fixr.order.orders;

import in.fixr.order.auth.CurrentUser;
import in.fixr.order.common.ApiException;
import in.fixr.order.user.Role;
import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Pattern;
import jakarta.validation.constraints.Size;
import java.util.List;
import java.util.UUID;
import org.springframework.data.domain.Limit;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/orders")
public class OrderController {
    private final OrderService service;
    private final OrderRepository orders;
    private final TransitionRepository transitions;
    private final CurrentUser current;

    public OrderController(OrderService service, OrderRepository orders, TransitionRepository transitions, CurrentUser current) {
        this.service = service;
        this.orders = orders;
        this.transitions = transitions;
        this.current = current;
    }

    public record NewOrder(@NotBlank @Size(max = 48) String serviceId, @NotNull UUID addressId, @Size(max = 500) String notes) {}

    @PostMapping
    public ResponseEntity<OrderView> create(@RequestHeader(name = "Idempotency-Key", required = false) String key,
                                            @Valid @RequestBody NewOrder body) {
        current.require(Role.CUSTOMER);
        if (key == null || key.isBlank() || key.length() > 64) {
            throw ApiException.badRequest("IDEMPOTENCY_KEY_REQUIRED", "Send an Idempotency-Key header (max 64 chars)");
        }
        String notes = body.notes() == null || body.notes().isBlank() ? null : body.notes().strip();
        var r = service.create(current.id(), key, body.serviceId(), body.addressId(), notes);
        return ResponseEntity.status(r.replayed() ? HttpStatus.OK : HttpStatus.CREATED).body(OrderView.forCustomer(r.order()));
    }

    @GetMapping
    public List<OrderView> mine() {
        UUID me = current.id();
        return current.role() == Role.PRO
                ? orders.findByProIdOrderByCreatedAtDesc(me, Limit.of(50)).stream().map(OrderView::forPro).toList()
                : orders.findByCustomerIdOrderByCreatedAtDesc(me, Limit.of(50)).stream().map(OrderView::forCustomer).toList();
    }

    @GetMapping("/{id}")
    public OrderView get(@PathVariable UUID id) {
        Order o = service.load(id);
        UUID me = current.id();
        if (o.getCustomerId().equals(me)) return OrderView.forCustomer(o);
        if (me.equals(o.getProId())) return OrderView.forPro(o);
        throw ApiException.notFound("Order");
    }

    public record TimelineEntry(String from, String to, String actor, String note, java.time.Instant at) {}

    @GetMapping("/{id}/timeline")
    public List<TimelineEntry> timeline(@PathVariable UUID id) {
        get(id);
        return transitions.findByOrderIdOrderByCreatedAtAsc(id).stream()
                .map(t -> new TimelineEntry(t.getFrom() == null ? null : t.getFrom().name(), t.getTo().name(),
                        t.getActor().name(), t.getNote(), t.getCreatedAt()))
                .toList();
    }

    public record Action(Long version, @Size(max = 200) String reason, @Pattern(regexp = "\\d{4}") String code) {}

    @PostMapping("/{id}/cancel")
    public OrderView cancel(@PathVariable UUID id, @Valid @RequestBody(required = false) Action body) {
        current.require(Role.CUSTOMER);
        Action a = body == null ? new Action(null, null, null) : body;
        return OrderView.forCustomer(service.cancelByCustomer(id, current.id(), a.version(), a.reason()));
    }

    @PostMapping("/{id}/{action:en-route|arrived|start|complete|drop}")
    public OrderView proAction(@PathVariable UUID id, @PathVariable String action, @Valid @RequestBody(required = false) Action body) {
        current.require(Role.PRO);
        Action a = body == null ? new Action(null, null, null) : body;
        var act = OrderService.ProAction.valueOf(action.toUpperCase().replace('-', '_'));
        return OrderView.forPro(service.proAction(id, current.id(), act, a.version(), a.code(), a.reason()));
    }
}
