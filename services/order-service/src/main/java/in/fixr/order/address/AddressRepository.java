package in.fixr.order.address;

import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface AddressRepository extends JpaRepository<Address, UUID> {
    List<Address> findByUserIdAndDeletedAtIsNullOrderByCreatedAtDesc(UUID userId);

    long countByUserIdAndDeletedAtIsNull(UUID userId);
}
