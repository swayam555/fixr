package in.fixr.order.auth;

import java.time.Duration;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Component;

/** Fixed-window counter in Redis. Nginx limits per IP; this limits per phone so rotating IPs can't spam one number. */
@Component
public class RateLimiter {
    private final StringRedisTemplate redis;

    public RateLimiter(StringRedisTemplate redis) {
        this.redis = redis;
    }

    public boolean tryAcquire(String key, int limit, Duration window) {
        Long n = redis.opsForValue().increment("rl:" + key);
        if (n != null && n == 1) redis.expire("rl:" + key, window);
        return n != null && n <= limit;
    }
}
