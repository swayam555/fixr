package in.fixr.order.orders;

import java.util.EnumSet;
import java.util.Set;

public enum OrderStatus {
    SEARCHING, ASSIGNED, EN_ROUTE, ARRIVED, IN_PROGRESS, COMPLETED, CANCELLED, UNFULFILLED;

    private Set<OrderStatus> next;

    static {
        SEARCHING.next = EnumSet.of(ASSIGNED, CANCELLED, UNFULFILLED);
        ASSIGNED.next = EnumSet.of(EN_ROUTE, CANCELLED, SEARCHING);
        EN_ROUTE.next = EnumSet.of(ARRIVED, CANCELLED, SEARCHING);
        ARRIVED.next = EnumSet.of(IN_PROGRESS, CANCELLED);
        IN_PROGRESS.next = EnumSet.of(COMPLETED);
        COMPLETED.next = EnumSet.noneOf(OrderStatus.class);
        CANCELLED.next = EnumSet.noneOf(OrderStatus.class);
        UNFULFILLED.next = EnumSet.noneOf(OrderStatus.class);
    }

    public boolean canMoveTo(OrderStatus to) {
        return next.contains(to);
    }

    public boolean isTerminal() {
        return next.isEmpty();
    }

    public boolean hasPro() {
        return this == ASSIGNED || this == EN_ROUTE || this == ARRIVED || this == IN_PROGRESS || this == COMPLETED;
    }
}
