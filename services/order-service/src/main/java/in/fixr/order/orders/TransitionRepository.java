package in.fixr.order.orders;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface TransitionRepository extends JpaRepository<Transition, UUID> {
    List<Transition> findByOrderIdOrderByCreatedAtAsc(UUID orderId);
}
