package in.fixr.order;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.webmvc.test.autoconfigure.AutoConfigureMockMvc;
import org.springframework.context.annotation.Import;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.test.web.servlet.MockMvc;

@SpringBootTest(properties = {"fixr.otp.dev-echo=true", "management.tracing.export.enabled=false"})
@AutoConfigureMockMvc
@Import(TestcontainersConfiguration.class)
public abstract class AbstractIT {
    @Autowired
    protected MockMvc mvc;

    @DynamicPropertySource
    static void keys(DynamicPropertyRegistry r) {
        r.add("fixr.jwt.private-key-path", TestKeys::privateKeyPath);
    }
}
