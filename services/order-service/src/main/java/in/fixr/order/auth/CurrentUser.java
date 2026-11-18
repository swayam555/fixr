package in.fixr.order.auth;

import in.fixr.order.common.ApiException;
import in.fixr.order.user.Role;
import java.util.UUID;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.stereotype.Component;

@Component
public class CurrentUser {
    public UUID id() {
        return UUID.fromString(jwt().getSubject());
    }

    public Role role() {
        return Role.valueOf(jwt().getClaimAsString("role"));
    }

    public void require(Role role) {
        if (role() != role) throw ApiException.forbidden("This action needs a " + role.name().toLowerCase() + " account");
    }

    private Jwt jwt() {
        if (SecurityContextHolder.getContext().getAuthentication() != null
                && SecurityContextHolder.getContext().getAuthentication().getPrincipal() instanceof Jwt jwt) {
            return jwt;
        }
        throw ApiException.forbidden("Not signed in");
    }
}
