package in.fixr.order.outbox;

import java.util.concurrent.atomic.AtomicInteger;
import org.springframework.stereotype.Component;

/** Lets tests and chaos drills make the next N publishes fail before they reach Kafka. */
@Component
public class PublishFaults {
    private final AtomicInteger failNext = new AtomicInteger();

    public void failNext(int n) {
        failNext.set(n);
    }

    boolean shouldFail() {
        return failNext.getAndUpdate(n -> Math.max(0, n - 1)) > 0;
    }
}
