import { normalisePhone } from '@fixr/core';
import { PhoneLogin } from '@fixr/ui';
import { auth, useSession } from '@/lib/api';

export default function Login() {
  return (
    <PhoneLogin
      copy={{
        eyebrow: 'FIXR PARTNER',
        title: 'More jobs, less waiting between them.',
        subtitle: 'Go online and we route nearby jobs to you, with the tools to carry and the time it should take.',
      }}
      validatePhone={normalisePhone}
      requestOtp={(phone) => auth.requestOtp(phone, 'PRO')}
      verifyOtp={async (id, otp) => {
        const s = await auth.verifyOtp(id, otp);
        useSession.getState().signIn({ accessToken: s.accessToken, refreshToken: s.refreshToken }, s.user);
      }}
    />
  );
}
