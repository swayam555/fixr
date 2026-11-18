package in.fixr.order.auth;

import in.fixr.order.common.Ids;
import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "refresh_tokens")
public class RefreshToken {
    @Id
    private UUID id;
    @Column(name = "user_id")
    private UUID userId;
    @Column(name = "family_id")
    private UUID familyId;
    @Column(name = "token_hash")
    private String tokenHash;
    @Column(name = "expires_at")
    private Instant expiresAt;
    @Column(name = "revoked_at")
    private Instant revokedAt;
    @Column(name = "replaced_by")
    private UUID replacedBy;
    @Column(name = "created_at")
    private Instant createdAt;

    protected RefreshToken() {}

    RefreshToken(UUID userId, UUID familyId, String tokenHash, Instant expiresAt) {
        this.id = Ids.newId();
        this.userId = userId;
        this.familyId = familyId;
        this.tokenHash = tokenHash;
        this.expiresAt = expiresAt;
        this.createdAt = Instant.now();
    }

    UUID getId() { return id; }
    UUID getUserId() { return userId; }
    UUID getFamilyId() { return familyId; }
    boolean isRevoked() { return revokedAt != null; }
    boolean isExpired() { return Instant.now().isAfter(expiresAt); }

    void revoke(UUID replacedBy) {
        this.revokedAt = Instant.now();
        this.replacedBy = replacedBy;
    }
}
