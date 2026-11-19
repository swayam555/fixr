package in.fixr.order.catalogue;

import java.util.Optional;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.stereotype.Component;

@Component
public class Zones {
    private final JdbcClient jdbc;

    public Zones(JdbcClient jdbc) {
        this.jdbc = jdbc;
    }

    public record Zone(String id, String name) {}

    public Optional<Zone> containing(double lat, double lng) {
        return jdbc.sql("""
                        SELECT id, name FROM zones
                        WHERE active AND ST_Contains(geom, ST_SetSRID(ST_MakePoint(:lng, :lat), 4326))
                        LIMIT 1""")
                .param("lat", lat).param("lng", lng)
                .query((rs, i) -> new Zone(rs.getString("id"), rs.getString("name")))
                .optional();
    }
}
