package in.fixr.order.common;

import java.util.regex.Pattern;

public final class Phones {
    private static final Pattern E164_IN = Pattern.compile("^\\+91[6-9]\\d{9}$");

    private Phones() {}

    public static boolean isValidIndianMobile(String phone) {
        return phone != null && E164_IN.matcher(phone).matches();
    }

    public static String mask(String phone) {
        if (phone == null || phone.length() < 6) return "••••";
        return phone.substring(0, 3) + " ••••• " + phone.substring(phone.length() - 3);
    }
}
