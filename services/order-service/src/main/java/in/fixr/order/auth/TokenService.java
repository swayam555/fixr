package in.fixr.order.auth;

import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import in.fixr.order.common.ApiException;
import in.fixr.order.common.Ids;
import in.fixr.order.user.User;
import java.security.SecureRandom;
import java.time.Instant;
import java.util.Base64;
import java.util.Date;
import java.util.HexFormat;
import java.util.UUID;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class TokenService {
    private static final SecureRandom RANDOM = new SecureRandom();
    private final JwtKeys keys;
    private final AuthProperties props;
    private final RefreshTokenRepository refreshTokens;

    public TokenService(JwtKeys keys, AuthProperties props, RefreshTokenRepository refreshTokens) {
        this.keys = keys;
        this.props = props;
        this.refreshTokens = refreshTokens;
    }

    public record Pair(String accessToken, String refreshToken) {}

    @Transactional
    public Pair issue(User user) {
        return new Pair(access(user), issueRefresh(user.getId(), Ids.newId()).token());
    }

    /** Rotating refresh tokens with reuse detection: presenting a revoked token kills the whole family. */
    @Transactional(noRollbackFor = ApiException.class)
    public RotateResult rotate(String presented) {
        RefreshToken current = refreshTokens.findByTokenHash(Hashing.sha256(presented))
                .orElseThrow(() -> invalid());
        if (current.isRevoked()) {
            refreshTokens.revokeFamily(current.getFamilyId(), Instant.now());
            throw invalid();
        }
        if (current.isExpired()) throw invalid();
        Issued next = issueRefresh(current.getUserId(), current.getFamilyId());
        current.revoke(next.entity().getId());
        return new RotateResult(current.getUserId(), next.token());
    }

    public record RotateResult(UUID userId, String refreshToken) {}

    @Transactional
    public void revoke(String presented) {
        refreshTokens.findByTokenHash(Hashing.sha256(presented))
                .ifPresent(t -> refreshTokens.revokeFamily(t.getFamilyId(), Instant.now()));
    }

    public String access(User user) {
        try {
            Instant now = Instant.now();
            JWTClaimsSet claims = new JWTClaimsSet.Builder()
                    .issuer(props.jwt().issuer())
                    .subject(user.getId().toString())
                    .claim("role", user.getRole().name())
                    .issueTime(Date.from(now))
                    .expirationTime(Date.from(now.plus(props.jwt().accessTtl())))
                    .jwtID(Ids.newId().toString())
                    .build();
            SignedJWT jwt = new SignedJWT(new JWSHeader.Builder(JWSAlgorithm.RS256).keyID(keys.jwk().getKeyID()).build(), claims);
            jwt.sign(new RSASSASigner(keys.jwk()));
            return jwt.serialize();
        } catch (Exception e) {
            throw new IllegalStateException("Could not sign JWT", e);
        }
    }

    private record Issued(String token, RefreshToken entity) {}

    private Issued issueRefresh(UUID userId, UUID familyId) {
        byte[] raw = new byte[32];
        RANDOM.nextBytes(raw);
        String token = Base64.getUrlEncoder().withoutPadding().encodeToString(raw);
        RefreshToken entity = refreshTokens.save(
                new RefreshToken(userId, familyId, Hashing.sha256(token), Instant.now().plus(props.jwt().refreshTtl())));
        return new Issued(token, entity);
    }

    private static ApiException invalid() {
        return new ApiException(HttpStatus.UNAUTHORIZED, "INVALID_REFRESH", "Please sign in again");
    }

    static final class Hashing {
        static String sha256(String s) {
            try {
                return HexFormat.of().formatHex(java.security.MessageDigest.getInstance("SHA-256").digest(s.getBytes()));
            } catch (Exception e) {
                throw new IllegalStateException(e);
            }
        }
    }
}
