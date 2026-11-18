export type Role = 'CUSTOMER' | 'PRO' | 'OPS';

export interface User {
  id: string;
  phone: string;
  maskedPhone: string;
  role: Role;
  name: string | null;
}

export interface OtpChallenge {
  challengeId: string;
  expiresInSec: number;
  resendInSec: number;
  devOtp?: string;
}

export interface Session {
  accessToken: string;
  refreshToken: string;
  user: User;
  isNewUser: boolean;
}
