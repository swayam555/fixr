package in.fixr.order.auth;

import in.fixr.order.common.Ids;
import in.fixr.order.user.Role;
import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "otp_challenges")
public class OtpChallenge {
    @Id
    private UUID id;
    private String phone;
    @Enumerated(EnumType.STRING)
    private Role role;
    @Column(name = "code_hash")
    private String codeHash;
    private int attempts;
    @Column(name = "expires_at")
    private Instant expiresAt;
    @Column(name = "consumed_at")
    private Instant consumedAt;
    @Column(name = "created_at")
    private Instant createdAt;

    protected OtpChallenge() {}

    OtpChallenge(String phone, Role role, Instant expiresAt) {
        this.id = Ids.newId();
        this.phone = phone;
        this.role = role;
        this.expiresAt = expiresAt;
        this.createdAt = Instant.now();
    }

    UUID getId() { return id; }
    String getPhone() { return phone; }
    Role getRole() { return role; }
    String getCodeHash() { return codeHash; }
    int getAttempts() { return attempts; }
    Instant getCreatedAt() { return createdAt; }
    boolean isUsable() { return consumedAt == null && Instant.now().isBefore(expiresAt); }

    void setCodeHash(String codeHash) { this.codeHash = codeHash; }
    void recordAttempt() { attempts++; }
    void consume() { consumedAt = Instant.now(); }
}
