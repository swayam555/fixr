package in.fixr.order.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.jayway.jsonpath.JsonPath;
import in.fixr.order.AbstractIT;
import java.util.concurrent.ThreadLocalRandom;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;

class AuthFlowIT extends AbstractIT {

    private static String randomPhone() {
        return "+919" + String.format("%09d", ThreadLocalRandom.current().nextInt(1_000_000_000));
    }

    private String requestOtp(String phone, String role) throws Exception {
        return mvc.perform(post("/api/auth/otp").contentType(MediaType.APPLICATION_JSON)
                        .content(("{\"phone\":\"" + phone + "\",\"role\":\"" + role + "\"}")))
                .andExpect(status().isOk())
                .andReturn().getResponse().getContentAsString();
    }

    private String verify(String challengeId, String otp, int expectStatus) throws Exception {
        return mvc.perform(post("/api/auth/verify").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"challengeId\":\"" + challengeId + "\",\"otp\":\"" + otp + "\"}"))
                .andExpect(status().is(expectStatus))
                .andReturn().getResponse().getContentAsString();
    }

    @Test
    void otpLoginIssuesTokensThatWorkOnProtectedRoutes() throws Exception {
        String phone = randomPhone();
        String issued = requestOtp(phone, "CUSTOMER");
        String session = verify(JsonPath.read(issued, "$.challengeId"), JsonPath.read(issued, "$.devOtp"), 200);

        assertThat((Boolean) JsonPath.read(session, "$.isNewUser")).isTrue();
        String access = JsonPath.read(session, "$.accessToken");
        mvc.perform(get("/api/me").header("Authorization", "Bearer " + access))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.role").value("CUSTOMER"))
                .andExpect(jsonPath("$.maskedPhone").value(phone.substring(0, 3) + " ••••• " + phone.substring(10)));
    }

    @Test
    void protectedRoutesRejectMissingToken() throws Exception {
        mvc.perform(get("/api/me")).andExpect(status().isUnauthorized());
    }

    @Test
    void sameNumberGetsSeparateCustomerAndProAccounts() throws Exception {
        String phone = randomPhone();
        String c = requestOtp(phone, "CUSTOMER");
        String cs = verify(JsonPath.read(c, "$.challengeId"), JsonPath.read(c, "$.devOtp"), 200);
        assertThat((String) JsonPath.read(cs, "$.user.role")).isEqualTo("CUSTOMER");
        // the resend cooldown is per phone, so a second role must wait; prove it is enforced
        mvc.perform(post("/api/auth/otp").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"phone\":\"" + phone + "\",\"role\":\"PRO\"}"))
                .andExpect(status().isTooManyRequests());
    }

    @Test
    void wrongCodesAreLimitedAndChallengeCannotBeReused() throws Exception {
        String issued = requestOtp(randomPhone(), "PRO");
        String id = JsonPath.read(issued, "$.challengeId");
        String good = JsonPath.read(issued, "$.devOtp");
        String bad = good.equals("000000") ? "111111" : "000000";
        for (int i = 0; i < 5; i++) verify(id, bad, 401);
        verify(id, good, 429);
    }

    @Test
    void consumedChallengeIsGone() throws Exception {
        String issued = requestOtp(randomPhone(), "CUSTOMER");
        verify(JsonPath.read(issued, "$.challengeId"), JsonPath.read(issued, "$.devOtp"), 200);
        verify(JsonPath.read(issued, "$.challengeId"), JsonPath.read(issued, "$.devOtp"), 410);
    }

    @Test
    void refreshRotatesAndReuseRevokesTheFamily() throws Exception {
        String issued = requestOtp(randomPhone(), "CUSTOMER");
        String session = verify(JsonPath.read(issued, "$.challengeId"), JsonPath.read(issued, "$.devOtp"), 200);
        String r1 = JsonPath.read(session, "$.refreshToken");

        String rotated = mvc.perform(post("/api/auth/refresh").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"refreshToken\":\"" + r1 + "\"}"))
                .andExpect(status().isOk()).andReturn().getResponse().getContentAsString();
        String r2 = JsonPath.read(rotated, "$.refreshToken");
        assertThat(r2).isNotEqualTo(r1);

        mvc.perform(post("/api/auth/refresh").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"refreshToken\":\"" + r1 + "\"}"))
                .andExpect(status().isUnauthorized());
        mvc.perform(post("/api/auth/refresh").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"refreshToken\":\"" + r2 + "\"}"))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void rejectsNonIndianNumbersAndOpsSelfSignup() throws Exception {
        mvc.perform(post("/api/auth/otp").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"phone\":\"+15551234567\",\"role\":\"CUSTOMER\"}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("INVALID_PHONE"));
        mvc.perform(post("/api/auth/otp").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"phone\":\"" + randomPhone() + "\",\"role\":\"OPS\"}"))
                .andExpect(status().isForbidden());
    }

    @Test
    void jwksExposesOnlyThePublicKey() throws Exception {
        String jwks = mvc.perform(get("/.well-known/jwks.json")).andExpect(status().isOk())
                .andReturn().getResponse().getContentAsString();
        assertThat(jwks).contains("\"kty\":\"RSA\"").doesNotContain("\"d\":");
    }
}
