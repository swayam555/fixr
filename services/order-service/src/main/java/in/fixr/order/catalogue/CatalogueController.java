package in.fixr.order.catalogue;

import in.fixr.order.common.ApiException;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;
import org.springframework.http.CacheControl;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/catalogue")
public class CatalogueController {
    private final CatalogueRepository repo;

    public CatalogueController(CatalogueRepository repo) {
        this.repo = repo;
    }

    public record ServiceView(String id, String categoryId, String name, String description, int pricePaise,
                              int estMinutes, int skillLevel, List<String> tools) {
        static ServiceView of(ServiceItem s) {
            return new ServiceView(s.getId(), s.getCategoryId(), s.getName(), s.getDescription(), s.getPricePaise(),
                    s.getEstMinutes(), s.getSkillLevel(), s.getTools());
        }
    }

    public record CategoryView(String id, String name, String icon, List<ServiceView> services) {}

    @GetMapping
    public ResponseEntity<List<CategoryView>> catalogue() {
        Map<String, List<ServiceView>> byCat = repo.findByActiveTrueOrderByCategoryIdAscSortAsc().stream()
                .map(ServiceView::of)
                .collect(Collectors.groupingBy(ServiceView::categoryId));
        List<CategoryView> body = repo.categories().stream()
                .filter(c -> byCat.containsKey(c.getId()))
                .map(c -> new CategoryView(c.getId(), c.getName(), c.getIcon(), byCat.get(c.getId())))
                .toList();
        return ResponseEntity.ok().cacheControl(CacheControl.maxAge(Duration.ofMinutes(5)).cachePublic()).body(body);
    }

    @GetMapping("/services/{id}")
    public ServiceView service(@PathVariable String id) {
        return repo.findById(id).filter(ServiceItem::isActive).map(ServiceView::of)
                .orElseThrow(() -> ApiException.notFound("Service"));
    }
}
