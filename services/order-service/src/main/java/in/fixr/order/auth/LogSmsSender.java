package in.fixr.order.auth;

import in.fixr.order.common.Phones;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.stereotype.Component;

/** Local stand-in for an SMS gateway (MSG91 / Gupshup adapter plugs in as another SmsSender bean). */
@Component
@ConditionalOnMissingBean(value = SmsSender.class, ignored = LogSmsSender.class)
public class LogSmsSender implements SmsSender {
    private static final Logger log = LoggerFactory.getLogger(LogSmsSender.class);

    @Override
    public void sendOtp(String phone, String code) {
        log.info("OTP for {} is {} (log SMS sender; configure a real gateway for production)", Phones.mask(phone), code);
    }
}
