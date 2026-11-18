package in.fixr.order;

import java.nio.file.Files;
import java.nio.file.Path;
import java.security.KeyPairGenerator;
import java.util.Base64;

public final class TestKeys {
    private static Path path;

    private TestKeys() {}

    public static synchronized String privateKeyPath() {
        if (path == null) {
            try {
                KeyPairGenerator g = KeyPairGenerator.getInstance("RSA");
                g.initialize(2048);
                String b64 = Base64.getMimeEncoder(64, "\n".getBytes()).encodeToString(g.generateKeyPair().getPrivate().getEncoded());
                path = Files.createTempFile("fixr-jwt", ".pem");
                Files.writeString(path, "-----BEGIN PRIVATE KEY-----\n" + b64 + "\n-----END PRIVATE KEY-----\n");
                path.toFile().deleteOnExit();
            } catch (Exception e) {
                throw new IllegalStateException(e);
            }
        }
        return path.toString();
    }
}
