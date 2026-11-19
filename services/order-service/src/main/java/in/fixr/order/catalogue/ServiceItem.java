package in.fixr.order.catalogue;

import jakarta.persistence.*;
import java.util.List;
import org.hibernate.annotations.Immutable;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

@Entity
@Immutable
@Table(name = "services")
public class ServiceItem {
    @Id
    private String id;
    @Column(name = "category_id")
    private String categoryId;
    private String name;
    private String description;
    private String skill;
    @Column(name = "skill_level")
    private int skillLevel;
    @Column(name = "price_paise")
    private int pricePaise;
    @Column(name = "est_minutes")
    private int estMinutes;
    @JdbcTypeCode(SqlTypes.ARRAY)
    private List<String> tools;
    private boolean active;
    private int sort;

    protected ServiceItem() {}

    public String getId() { return id; }
    public String getCategoryId() { return categoryId; }
    public String getName() { return name; }
    public String getDescription() { return description; }
    public String getSkill() { return skill; }
    public int getSkillLevel() { return skillLevel; }
    public int getPricePaise() { return pricePaise; }
    public int getEstMinutes() { return estMinutes; }
    public List<String> getTools() { return tools; }
    public boolean isActive() { return active; }
    public int getSort() { return sort; }

    /** Shown to pros on the offer card: a band, not the exact price, until they accept. */
    public String priceBand() {
        int rupees = pricePaise / 100;
        int lo = rupees / 100 * 100;
        return "₹" + lo + "–" + (lo + 100);
    }
}
