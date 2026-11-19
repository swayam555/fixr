package in.fixr.order.catalogue;

import jakarta.validation.constraints.DecimalMax;
import jakarta.validation.constraints.DecimalMin;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.*;

@RestController
@Validated
@RequestMapping("/api/zones")
public class ZoneController {
    private final Zones zones;

    public ZoneController(Zones zones) {
        this.zones = zones;
    }

    public record Serviceable(boolean serviceable, String zoneId, String zoneName) {}

    @GetMapping("/serviceable")
    public Serviceable serviceable(@RequestParam @DecimalMin("-90") @DecimalMax("90") double lat,
                                   @RequestParam @DecimalMin("-180") @DecimalMax("180") double lng) {
        return zones.containing(lat, lng)
                .map(z -> new Serviceable(true, z.id(), z.name()))
                .orElse(new Serviceable(false, null, null));
    }
}
