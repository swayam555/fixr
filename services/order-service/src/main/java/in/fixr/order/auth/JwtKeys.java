package in.fixr.order.auth;

import com.nimbusds.jose.jwk.RSAKey;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.KeyFactory;
import java.security.interfaces.RSAPrivateCrtKey;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.RSAPublicKeySpec;
import java.util.Base64;
import org.springframework.stereotype.Component;

/** Loads the RS256 signing key shared by every order-service instance; Go services verify via /.well-known/jwks.json. */
@Component
public class JwtKeys {
    private final RSAKey jwk;

    public JwtKeys(AuthProperties props) throws Exception {
        String pem = Files.readString(Path.of(props.jwt().privateKeyPath()));
        String body = pem.replaceAll("-----[A-Z ]+-----", "").replaceAll("\\s", "");
        KeyFactory kf = KeyFactory.getInstance("RSA");
        RSAPrivateCrtKey priv = (RSAPrivateCrtKey) kf.generatePrivate(new PKCS8EncodedKeySpec(Base64.getDecoder().decode(body)));
        RSAPublicKey pub = (RSAPublicKey) kf.generatePublic(new RSAPublicKeySpec(priv.getModulus(), priv.getPublicExponent()));
        RSAKey unnamed = new RSAKey.Builder(pub).privateKey(priv).build();
        this.jwk = new RSAKey.Builder(pub).privateKey(priv).keyID(unnamed.computeThumbprint().toString()).build();
    }

    public RSAKey jwk() { return jwk; }

    public RSAPublicKey publicKey() throws Exception { return jwk.toRSAPublicKey(); }
}
