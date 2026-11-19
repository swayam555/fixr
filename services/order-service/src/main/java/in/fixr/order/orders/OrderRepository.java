package in.fixr.order.orders;

import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.domain.Limit;
import org.springframework.data.jpa.repository.JpaRepository;

public interface OrderRepository extends JpaRepository<Order, UUID> {
    Optional<Order> findByCustomerIdAndIdempotencyKey(UUID customerId, String key);

    List<Order> findByCustomerIdOrderByCreatedAtDesc(UUID customerId, Limit limit);

    List<Order> findByProIdOrderByCreatedAtDesc(UUID proId, Limit limit);

    long countByCustomerIdAndStatusIn(UUID customerId, List<OrderStatus> statuses);
}
