package in.fixr.order.auth;

import in.fixr.order.common.ApiException;
import in.fixr.order.user.Role;
import in.fixr.order.user.UserRepository;
import in.fixr.order.user.UserView;
import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Pattern;
import java.util.Map;
import java.util.UUID;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/auth")
public class AuthController {
    private final OtpService otp;
    private final TokenService tokens;
    private final UserRepository users;

    public AuthController(OtpService otp, TokenService tokens, UserRepository users) {
        this.otp = otp;
        this.tokens = tokens;
        this.users = users;
    }

    public record OtpRequest(@NotBlank String phone, @NotNull Role role) {}

    @PostMapping("/otp")
    public Map<String, Object> requestOtp(@Valid @RequestBody OtpRequest body) {
        OtpService.Issued i = otp.request(body.phone(), body.role());
        var out = new java.util.LinkedHashMap<String, Object>();
        out.put("challengeId", i.challengeId());
        out.put("expiresInSec", i.expiresInSec());
        out.put("resendInSec", i.resendInSec());
        if (i.devOtp() != null) out.put("devOtp", i.devOtp());
        return out;
    }

    public record VerifyRequest(@NotNull UUID challengeId, @NotBlank @Pattern(regexp = "\\d{6}") String otp) {}

    public record SessionView(String accessToken, String refreshToken, UserView user, boolean isNewUser) {}

    @PostMapping("/verify")
    public SessionView verify(@Valid @RequestBody VerifyRequest body) {
        OtpService.Verified v = otp.verify(body.challengeId(), body.otp());
        return new SessionView(v.tokens().accessToken(), v.tokens().refreshToken(), UserView.of(v.user()), v.isNewUser());
    }

    public record RefreshRequest(@NotBlank String refreshToken) {}

    @PostMapping("/refresh")
    public SessionView refresh(@Valid @RequestBody RefreshRequest body) {
        TokenService.RotateResult r = tokens.rotate(body.refreshToken());
        var user = users.findById(r.userId())
                .orElseThrow(() -> new ApiException(HttpStatus.UNAUTHORIZED, "INVALID_REFRESH", "Please sign in again"));
        return new SessionView(tokens.access(user), r.refreshToken(), UserView.of(user), false);
    }

    @PostMapping("/logout")
    public ResponseEntity<Void> logout(@Valid @RequestBody RefreshRequest body) {
        tokens.revoke(body.refreshToken());
        return ResponseEntity.noContent().build();
    }
}
