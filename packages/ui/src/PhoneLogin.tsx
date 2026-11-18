import { useEffect, useState } from 'react';
import { KeyboardAvoidingView, Platform, View } from 'react-native';
import Animated, { FadeInDown, FadeOutUp } from 'react-native-reanimated';
import { OtpInput } from './OtpInput';
import { Button, Field, Screen, Stack, Text } from './primitives';
import { useTheme } from './theme';

export interface PhoneLoginCopy {
  eyebrow: string;
  title: string;
  subtitle: string;
}

export interface PhoneLoginProps {
  copy: PhoneLoginCopy;
  requestOtp(phone: string): Promise<{ challengeId: string; resendInSec: number; devOtp?: string }>;
  verifyOtp(challengeId: string, otp: string): Promise<void>;
  validatePhone(raw: string): string | null;
}

export function PhoneLogin({ copy, requestOtp, verifyOtp, validatePhone }: PhoneLoginProps) {
  const { space, c } = useTheme();
  const [phone, setPhone] = useState('');
  const [challenge, setChallenge] = useState<{ id: string; phone: string; devOtp?: string } | null>(null);
  const [otp, setOtp] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [resendIn, setResendIn] = useState(0);

  useEffect(() => {
    if (resendIn <= 0) return;
    const t = setTimeout(() => setResendIn((s) => s - 1), 1000);
    return () => clearTimeout(t);
  }, [resendIn]);

  async function send(raw = phone) {
    const normalised = validatePhone(raw);
    if (!normalised) {
      setError('Enter a 10-digit Indian mobile number');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const res = await requestOtp(normalised);
      setChallenge({ id: res.challengeId, phone: normalised, devOtp: res.devOtp });
      setResendIn(res.resendInSec);
      setOtp('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not send the code');
    } finally {
      setBusy(false);
    }
  }

  async function verify(code = otp) {
    if (!challenge || code.length !== 6) return;
    setBusy(true);
    setError(null);
    try {
      await verifyOtp(challenge.id, code);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'That code did not work');
      setOtp('');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Screen>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1, justifyContent: 'space-between', paddingVertical: space.xl }}>
        <Stack gap="sm" style={{ marginTop: space.xxxl }}>
          <View style={{ alignSelf: 'flex-start', backgroundColor: c.accent, paddingHorizontal: space.sm, paddingVertical: 2, borderRadius: 6 }}>
            <Text variant="label" tone="accent">{copy.eyebrow}</Text>
          </View>
          <Text variant="hero">{copy.title}</Text>
          <Text tone="muted">{copy.subtitle}</Text>
        </Stack>

        {!challenge ? (
          <Animated.View key="phone" entering={FadeInDown} exiting={FadeOutUp}>
            <Stack gap="lg">
              <Field
                testID="phone-input"
                label="Mobile number"
                prefix="+91"
                keyboardType="phone-pad"
                autoComplete="tel"
                maxLength={11}
                value={phone}
                onChangeText={setPhone}
                onSubmitEditing={() => send()}
                error={error}
                placeholder="98765 43210"
              />
              <Button testID="send-otp" title="Send code" loading={busy} onPress={() => send()} />
            </Stack>
          </Animated.View>
        ) : (
          <Animated.View key="otp" entering={FadeInDown} exiting={FadeOutUp}>
            <Stack gap="lg">
              <Text tone="muted">Code sent to {challenge.phone.replace(/^\+91(\d{5})(\d{5})$/, '+91 $1 $2')}</Text>
              <OtpInput
                testID="otp-input"
                autoFocus
                value={otp}
                onChange={(v) => {
                  setOtp(v);
                  if (v.length === 6) verify(v);
                }}
              />
              {challenge.devOtp ? <Text variant="caption" tone="muted">Dev build: code is {challenge.devOtp}</Text> : null}
              {error ? <Text variant="caption" tone="danger">{error}</Text> : null}
              <Button testID="verify-otp" title="Verify" loading={busy} disabled={otp.length !== 6} onPress={() => verify()} />
              <Stack row justify="space-between">
                <Button kind="ghost" title="Change number" onPress={() => setChallenge(null)} />
                <Button kind="ghost" title={resendIn > 0 ? `Resend in ${resendIn}s` : 'Resend code'} disabled={resendIn > 0} onPress={() => send(challenge.phone)} />
              </Stack>
            </Stack>
          </Animated.View>
        )}
      </KeyboardAvoidingView>
    </Screen>
  );
}
