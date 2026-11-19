package in.fixr.order.catalogue;

import java.util.List;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;

public interface CatalogueRepository extends JpaRepository<ServiceItem, String> {
    List<ServiceItem> findByActiveTrueOrderByCategoryIdAscSortAsc();

    @Query("select c from ServiceCategory c order by c.sort")
    List<ServiceCategory> categories();
}
