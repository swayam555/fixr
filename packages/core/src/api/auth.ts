import type { ApiClient } from './client';
import type { OtpChallenge, Role, Session, User } from './types';

export function authApi(api: ApiClient) {
  return {
    requestOtp: (phone: string, role: Role) =>
      api.post<OtpChallenge>('/api/auth/otp', { phone, role }, { auth: false }),
    verifyOtp: (challengeId: string, otp: string) =>
      api.post<Session>('/api/auth/verify', { challengeId, otp }, { auth: false }),
    me: () => api.get<User>('/api/me'),
    updateProfile: (name: string) => api.put<User>('/api/me', { name }),
    logout: (refreshToken: string) => api.post<void>('/api/auth/logout', { refreshToken }),
  };
}

export const INDIAN_MOBILE = /^[6-9]\d{9}$/;

export function normalisePhone(input: string): string | null {
  const digits = input.replace(/\D/g, '').replace(/^(91|0)(?=[6-9]\d{9}$)/, '');
  return INDIAN_MOBILE.test(digits) ? `+91${digits}` : null;
}
