package in.fixr.order.auth;

import java.time.Duration;
import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "fixr")
public record AuthProperties(Jwt jwt, Otp otp) {
    public record Jwt(String privateKeyPath, String issuer, Duration accessTtl, Duration refreshTtl) {}

    public record Otp(String secret, Duration ttl, Duration resendAfter, int maxAttempts, int perPhoneLimit,
                      Duration perPhoneWindow, boolean devEcho) {}
}
