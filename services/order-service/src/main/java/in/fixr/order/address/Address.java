package in.fixr.order.address;

import in.fixr.order.common.Ids;
import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "addresses")
public class Address {
    @Id
    private UUID id;
    @Column(name = "user_id")
    private UUID userId;
    private String label;
    private String line1;
    private String society;
    private String flat;
    private String landmark;
    private double lat;
    private double lng;
    @Column(name = "zone_id")
    private String zoneId;
    @Column(name = "deleted_at")
    private Instant deletedAt;
    @Column(name = "created_at")
    private Instant createdAt;

    protected Address() {}

    public Address(UUID userId, String label, String line1, String society, String flat, String landmark,
                   double lat, double lng, String zoneId) {
        this.id = Ids.newId();
        this.userId = userId;
        this.label = label;
        this.line1 = line1;
        this.society = society;
        this.flat = flat;
        this.landmark = landmark;
        this.lat = lat;
        this.lng = lng;
        this.zoneId = zoneId;
        this.createdAt = Instant.now();
    }

    public UUID getId() { return id; }
    public UUID getUserId() { return userId; }
    public String getLabel() { return label; }
    public String getLine1() { return line1; }
    public String getSociety() { return society; }
    public String getFlat() { return flat; }
    public String getLandmark() { return landmark; }
    public double getLat() { return lat; }
    public double getLng() { return lng; }
    public String getZoneId() { return zoneId; }
    public boolean isDeleted() { return deletedAt != null; }

    public void delete() { this.deletedAt = Instant.now(); }

    /** What a pro sees before arrival: society and area, never the flat number. */
    public String summary() {
        String place = society != null && !society.isBlank() ? society + ", " : "";
        return (place + line1).length() > 200 ? (place + line1).substring(0, 200) : place + line1;
    }
}
