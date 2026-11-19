package in.fixr.order.address;

import in.fixr.order.auth.CurrentUser;
import in.fixr.order.catalogue.Zones;
import in.fixr.order.common.ApiException;
import in.fixr.order.user.Role;
import jakarta.validation.Valid;
import jakarta.validation.constraints.*;
import java.util.List;
import java.util.UUID;
import org.springframework.http.HttpStatus;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/addresses")
public class AddressController {
    private static final int MAX_ADDRESSES = 20;

    private final AddressRepository repo;
    private final Zones zones;
    private final CurrentUser current;

    public AddressController(AddressRepository repo, Zones zones, CurrentUser current) {
        this.repo = repo;
        this.zones = zones;
        this.current = current;
    }

    public record AddressView(String id, String label, String line1, String society, String flat, String landmark,
                              double lat, double lng, String zoneId, boolean serviceable) {
        static AddressView of(Address a) {
            return new AddressView(a.getId().toString(), a.getLabel(), a.getLine1(), a.getSociety(), a.getFlat(),
                    a.getLandmark(), a.getLat(), a.getLng(), a.getZoneId(), a.getZoneId() != null);
        }
    }

    public record NewAddress(@NotBlank @Size(max = 24) String label,
                             @NotBlank @Size(max = 160) String line1,
                             @Size(max = 80) String society,
                             @Size(max = 32) String flat,
                             @Size(max = 80) String landmark,
                             @DecimalMin("-90") @DecimalMax("90") double lat,
                             @DecimalMin("-180") @DecimalMax("180") double lng) {}

    @GetMapping
    public List<AddressView> list() {
        return repo.findByUserIdAndDeletedAtIsNullOrderByCreatedAtDesc(current.id()).stream().map(AddressView::of).toList();
    }

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    @Transactional
    public AddressView create(@Valid @RequestBody NewAddress b) {
        current.require(Role.CUSTOMER);
        UUID uid = current.id();
        if (repo.countByUserIdAndDeletedAtIsNull(uid) >= MAX_ADDRESSES) {
            throw ApiException.badRequest("TOO_MANY_ADDRESSES", "Remove an old address before adding another");
        }
        String zone = zones.containing(b.lat(), b.lng()).map(Zones.Zone::id).orElse(null);
        Address a = new Address(uid, b.label().strip(), b.line1().strip(), trim(b.society()), trim(b.flat()),
                trim(b.landmark()), b.lat(), b.lng(), zone);
        return AddressView.of(repo.save(a));
    }

    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    @Transactional
    public void delete(@PathVariable UUID id) {
        Address a = repo.findById(id).filter(x -> x.getUserId().equals(current.id()) && !x.isDeleted())
                .orElseThrow(() -> ApiException.notFound("Address"));
        a.delete();
    }

    private static String trim(String s) {
        return s == null || s.isBlank() ? null : s.strip();
    }
}
