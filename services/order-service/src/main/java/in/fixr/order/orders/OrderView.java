package in.fixr.order.orders;

import java.time.Instant;

/** One view type for both apps; fields a role must not see are nulled by {@link #forPro}. */
public record OrderView(String id, String status, long version, String serviceId, String zoneId, String addressId,
                        String addressSummary, double lat, double lng, int pricePaise, String notes, String startCode,
                        String proId, Integer etaSec, String cancelReason, Instant createdAt, Instant assignedAt,
                        Instant arrivedAt, Instant startedAt, Instant completedAt, Instant closedAt) {

    public static OrderView forCustomer(Order o) {
        boolean showCode = !o.getStatus().isTerminal() && o.getStatus() != OrderStatus.IN_PROGRESS;
        return of(o, showCode ? o.getStartCode() : null);
    }

    public static OrderView forPro(Order o) {
        return of(o, null);
    }

    private static OrderView of(Order o, String code) {
        return new OrderView(o.getId().toString(), o.getStatus().name(), o.getVersion(), o.getServiceId(), o.getZoneId(),
                o.getAddressId().toString(), o.getAddressSummary(), o.getLat(), o.getLng(), o.getPricePaise(), o.getNotes(),
                code, o.getProId() == null ? null : o.getProId().toString(), o.getEtaSec(), o.getCancelReason(),
                o.getCreatedAt(), o.getAssignedAt(), o.getArrivedAt(), o.getStartedAt(), o.getCompletedAt(), o.getClosedAt());
    }
}
