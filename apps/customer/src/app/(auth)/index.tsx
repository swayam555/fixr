import { normalisePhone } from '@fixr/core';
import { PhoneLogin } from '@fixr/ui';
import { auth, useSession } from '@/lib/api';

export default function Login() {
  return (
    <PhoneLogin
      copy={{
        eyebrow: 'FIXR · 10-MINUTE HOME HELP',
        title: 'A verified pro at your door before the chai cools.',
        subtitle: 'Leaky tap, dishes, a fan that won’t spin. Snap it, and we send the right person with the right tools.',
      }}
      validatePhone={normalisePhone}
      requestOtp={(phone) => auth.requestOtp(phone, 'CUSTOMER')}
      verifyOtp={async (id, otp) => {
        const s = await auth.verifyOtp(id, otp);
        useSession.getState().signIn({ accessToken: s.accessToken, refreshToken: s.refreshToken }, s.user);
      }}
    />
  );
}
