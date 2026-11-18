package in.fixr.order.user;

import in.fixr.order.auth.CurrentUser;
import in.fixr.order.common.ApiException;
import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/me")
public class MeController {
    private final UserRepository users;
    private final CurrentUser current;

    public MeController(UserRepository users, CurrentUser current) {
        this.users = users;
        this.current = current;
    }

    @GetMapping
    public UserView me() {
        return UserView.of(load());
    }

    public record UpdateMe(@NotBlank @Size(max = 80) String name) {}

    @PutMapping
    @Transactional
    public UserView update(@Valid @RequestBody UpdateMe body) {
        User u = load();
        u.rename(body.name().strip());
        return UserView.of(u);
    }

    private User load() {
        return users.findById(current.id()).orElseThrow(() -> ApiException.notFound("User"));
    }
}
