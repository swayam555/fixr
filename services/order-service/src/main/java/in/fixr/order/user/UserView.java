package in.fixr.order.user;

import in.fixr.order.common.Phones;

public record UserView(String id, String phone, String maskedPhone, Role role, String name) {
    public static UserView of(User u) {
        return new UserView(u.getId().toString(), u.getPhone(), Phones.mask(u.getPhone()), u.getRole(), u.getName());
    }
}
