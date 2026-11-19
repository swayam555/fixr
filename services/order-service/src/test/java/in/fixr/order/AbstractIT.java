package in.fixr.order;

import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.jayway.jsonpath.JsonPath;
import java.util.concurrent.ThreadLocalRandom;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.webmvc.test.autoconfigure.AutoConfigureMockMvc;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.test.web.servlet.MockMvc;

@SpringBootTest(properties = {"fixr.otp.dev-echo=true", "management.tracing.export.enabled=false", "fixr.outbox.poll-ms=50"})
@AutoConfigureMockMvc
@Import(TestcontainersConfiguration.class)
public abstract class AbstractIT {
    @Autowired
    protected MockMvc mvc;

    @DynamicPropertySource
    static void keys(DynamicPropertyRegistry r) {
        r.add("fixr.jwt.private-key-path", TestKeys::privateKeyPath);
    }

    protected static String randomPhone() {
        return "+919" + String.format("%09d", ThreadLocalRandom.current().nextInt(1_000_000_000));
    }

    public record Session(String userId, String bearer) {}

    protected Session login(String role) throws Exception {
        String issued = mvc.perform(post("/api/auth/otp").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"phone\":\"" + randomPhone() + "\",\"role\":\"" + role + "\"}"))
                .andExpect(status().isOk()).andReturn().getResponse().getContentAsString();
        String session = mvc.perform(post("/api/auth/verify").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"challengeId\":\"" + JsonPath.read(issued, "$.challengeId") + "\",\"otp\":\""
                                + JsonPath.read(issued, "$.devOtp") + "\"}"))
                .andExpect(status().isOk()).andReturn().getResponse().getContentAsString();
        return new Session(JsonPath.read(session, "$.user.id"), "Bearer " + JsonPath.read(session, "$.accessToken"));
    }
}
