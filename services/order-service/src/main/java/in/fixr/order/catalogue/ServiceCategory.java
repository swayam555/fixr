package in.fixr.order.catalogue;

import jakarta.persistence.*;
import org.hibernate.annotations.Immutable;

@Entity
@Immutable
@Table(name = "service_categories")
public class ServiceCategory {
    @Id
    private String id;
    private String name;
    private String icon;
    private int sort;

    protected ServiceCategory() {}

    public String getId() { return id; }
    public String getName() { return name; }
    public String getIcon() { return icon; }
    public int getSort() { return sort; }
}
