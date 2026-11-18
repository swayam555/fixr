package in.fixr.order.auth;

import in.fixr.order.common.ApiException;
import in.fixr.order.common.Phones;
import in.fixr.order.user.Role;
import in.fixr.order.user.User;
import in.fixr.order.user.UserRepository;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.time.Duration;
import java.time.Instant;
import java.util.HexFormat;
import java.util.UUID;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class OtpService {
    private static final SecureRandom RANDOM = new SecureRandom();
    private final OtpChallengeRepository challenges;
    private final UserRepository users;
    private final TokenService tokens;
    private final SmsSender sms;
    private final RateLimiter limiter;
    private final AuthProperties.Otp cfg;

    public OtpService(OtpChallengeRepository challenges, UserRepository users, TokenService tokens, SmsSender sms,
                      RateLimiter limiter, AuthProperties props) {
        this.challenges = challenges;
        this.users = users;
        this.tokens = tokens;
        this.sms = sms;
        this.limiter = limiter;
        this.cfg = props.otp();
    }

    public record Issued(UUID challengeId, long expiresInSec, long resendInSec, String devOtp) {}

    @Transactional
    public Issued request(String phone, Role role) {
        if (!Phones.isValidIndianMobile(phone)) throw ApiException.badRequest("INVALID_PHONE", "Enter a valid Indian mobile number");
        if (role == Role.OPS) throw ApiException.forbidden("Ops accounts are provisioned, not self-registered");
        challenges.findFirstByPhoneOrderByCreatedAtDesc(phone).ifPresent(last -> {
            Duration since = Duration.between(last.getCreatedAt(), Instant.now());
            if (since.compareTo(cfg.resendAfter()) < 0) {
                throw ApiException.tooMany("Wait " + (cfg.resendAfter().minus(since).toSeconds() + 1) + "s before asking for another code");
            }
        });
        if (!limiter.tryAcquire("otp:" + phone, cfg.perPhoneLimit(), cfg.perPhoneWindow())) {
            throw ApiException.tooMany("Too many codes requested. Try again later.");
        }
        String code = String.format("%06d", RANDOM.nextInt(1_000_000));
        OtpChallenge c = new OtpChallenge(phone, role, Instant.now().plus(cfg.ttl()));
        c.setCodeHash(hash(c.getId(), code));
        challenges.save(c);
        sms.sendOtp(phone, code);
        return new Issued(c.getId(), cfg.ttl().toSeconds(), cfg.resendAfter().toSeconds(), cfg.devEcho() ? code : null);
    }

    public record Verified(User user, TokenService.Pair tokens, boolean isNewUser) {}

    @Transactional(noRollbackFor = ApiException.class)
    public Verified verify(UUID challengeId, String code) {
        OtpChallenge c = challenges.findById(challengeId).orElseThrow(OtpService::wrong);
        if (!c.isUsable()) throw new ApiException(HttpStatus.GONE, "OTP_EXPIRED", "This code has expired. Request a new one.");
        if (c.getAttempts() >= cfg.maxAttempts()) throw ApiException.tooMany("Too many wrong attempts. Request a new code.");
        c.recordAttempt();
        boolean match = MessageDigest.isEqual(
                c.getCodeHash().getBytes(StandardCharsets.US_ASCII),
                hash(c.getId(), code).getBytes(StandardCharsets.US_ASCII));
        if (!match) throw wrong();
        c.consume();
        var existing = users.findByPhoneAndRole(c.getPhone(), c.getRole());
        User user = existing.orElseGet(() -> users.save(new User(c.getPhone(), c.getRole())));
        return new Verified(user, tokens.issue(user), existing.isEmpty());
    }

    private String hash(UUID challengeId, String code) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(cfg.secret().getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
            return HexFormat.of().formatHex(mac.doFinal((challengeId + ":" + code).getBytes(StandardCharsets.UTF_8)));
        } catch (Exception e) {
            throw new IllegalStateException(e);
        }
    }

    private static ApiException wrong() {
        return new ApiException(HttpStatus.UNAUTHORIZED, "OTP_INVALID", "That code is not right");
    }
}
