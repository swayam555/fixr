package in.fixr.order.user;

import in.fixr.order.common.Ids;
import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "users")
public class User {
    @Id
    private UUID id;
    private String phone;
    @Enumerated(EnumType.STRING)
    private Role role;
    private String name;
    @Column(name = "created_at")
    private Instant createdAt;
    @Column(name = "updated_at")
    private Instant updatedAt;

    protected User() {}

    public User(String phone, Role role) {
        this.id = Ids.newId();
        this.phone = phone;
        this.role = role;
        this.createdAt = Instant.now();
        this.updatedAt = createdAt;
    }

    public UUID getId() { return id; }
    public String getPhone() { return phone; }
    public Role getRole() { return role; }
    public String getName() { return name; }

    public void rename(String name) {
        this.name = name;
        this.updatedAt = Instant.now();
    }
}
