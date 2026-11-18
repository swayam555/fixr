package in.fixr.order.common;

import java.security.SecureRandom;
import java.util.UUID;

/** UUIDv7: time-ordered, so B-tree inserts stay append-mostly. */
public final class Ids {
    private static final SecureRandom RANDOM = new SecureRandom();

    private Ids() {}

    public static UUID newId() {
        long ms = System.currentTimeMillis();
        long rand = RANDOM.nextLong();
        long msb = (ms << 16) | 0x7000L | (rand >>> 52 & 0x0FFFL);
        long lsb = (RANDOM.nextLong() & 0x3FFFFFFFFFFFFFFFL) | 0x8000000000000000L;
        return new UUID(msb, lsb);
    }
}
