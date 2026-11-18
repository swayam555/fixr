import { useRef, type ComponentRef } from 'react';
import { Pressable, TextInput, View } from 'react-native';
import { Text } from './primitives';
import { useTheme } from './theme';

export function OtpInput({
  length = 6,
  value,
  onChange,
  autoFocus,
  testID,
}: {
  length?: number;
  value: string;
  onChange: (v: string) => void;
  autoFocus?: boolean;
  testID?: string;
}) {
  const { c, radius, space } = useTheme();
  const input = useRef<ComponentRef<typeof TextInput>>(null);
  const digits = Array.from({ length }, (_, i) => value[i] ?? '');
  return (
    <Pressable onPress={() => input.current?.focus()} accessibilityLabel="One-time password">
      <View style={{ flexDirection: 'row', gap: space.sm, justifyContent: 'space-between' }}>
        {digits.map((d, i) => {
          const active = i === Math.min(value.length, length - 1);
          return (
            <View
              key={i}
              style={{
                flex: 1,
                aspectRatio: 0.82,
                borderRadius: radius.md,
                backgroundColor: c.surface,
                borderWidth: active ? 2 : 1,
                borderColor: active ? c.text : c.border,
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <Text variant="title">{d}</Text>
            </View>
          );
        })}
      </View>
      <TextInput
        ref={input}
        testID={testID}
        value={value}
        onChangeText={(t) => onChange(t.replace(/\D/g, '').slice(0, length))}
        keyboardType="number-pad"
        textContentType="oneTimeCode"
        autoComplete="sms-otp"
        autoFocus={autoFocus}
        maxLength={length}
        style={{ position: 'absolute', opacity: 0.01, width: 1, height: 1 }}
      />
    </Pressable>
  );
}
