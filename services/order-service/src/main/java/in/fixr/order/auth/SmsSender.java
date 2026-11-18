package in.fixr.order.auth;

public interface SmsSender {
    void sendOtp(String phone, String code);
}
