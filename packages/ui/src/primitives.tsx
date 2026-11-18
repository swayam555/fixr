import type { ComponentRef, ReactNode, Ref } from 'react';
import {
  ActivityIndicator,
  Pressable,
  Text as RNText,
  TextInput,
  View,
  type PressableProps,
  type StyleProp,
  type TextInputProps,
  type TextProps,
  type TextStyle,
  type ViewProps,
  type ViewStyle,
} from 'react-native';
import Animated, { useAnimatedStyle, useSharedValue, withSpring } from 'react-native-reanimated';
import { SafeAreaView, type Edge } from 'react-native-safe-area-context';
import { useTheme } from './theme';
import { motion, type TypeVariant } from './tokens';

type Tone = 'default' | 'muted' | 'inverse' | 'accent' | 'danger' | 'success';

export function Text({
  variant = 'body',
  tone = 'default',
  style,
  ...rest
}: TextProps & { variant?: TypeVariant; tone?: Tone }) {
  const { c, type } = useTheme();
  const color = {
    default: c.text,
    muted: c.textMuted,
    inverse: c.textInverse,
    accent: c.accentText,
    danger: c.danger,
    success: c.success,
  }[tone];
  return <RNText {...rest} style={[type[variant], { color }, style]} />;
}

export function Screen({
  children,
  edges = ['top', 'bottom'],
  padded = true,
  style,
}: {
  children: ReactNode;
  edges?: Edge[];
  padded?: boolean;
  style?: StyleProp<ViewStyle>;
}) {
  const { c, space } = useTheme();
  return (
    <SafeAreaView edges={edges} style={[{ flex: 1, backgroundColor: c.bg }, padded && { paddingHorizontal: space.lg }, style]}>
      {children}
    </SafeAreaView>
  );
}

export function Stack({
  gap = 'md',
  row,
  align,
  justify,
  style,
  ...rest
}: ViewProps & {
  gap?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
  row?: boolean;
  align?: ViewStyle['alignItems'];
  justify?: ViewStyle['justifyContent'];
}) {
  const { space } = useTheme();
  return (
    <View
      {...rest}
      style={[{ gap: space[gap], flexDirection: row ? 'row' : 'column', alignItems: align, justifyContent: justify }, style]}
    />
  );
}

export function Card({ style, elevated, ...rest }: ViewProps & { elevated?: boolean }) {
  const { c, radius, space } = useTheme();
  return (
    <View
      {...rest}
      style={[
        {
          backgroundColor: c.surface,
          borderRadius: radius.lg,
          padding: space.lg,
          borderWidth: 1,
          borderColor: c.border,
        },
        elevated && { shadowColor: '#000', shadowOpacity: 0.08, shadowRadius: 16, shadowOffset: { width: 0, height: 6 }, elevation: 3 },
        style,
      ]}
    />
  );
}

type ButtonKind = 'primary' | 'secondary' | 'ghost' | 'danger';

export function Button({
  title,
  kind = 'primary',
  loading,
  disabled,
  icon,
  style,
  ...rest
}: Omit<PressableProps, 'style' | 'children'> & {
  title: string;
  kind?: ButtonKind;
  loading?: boolean;
  icon?: ReactNode;
  style?: StyleProp<ViewStyle>;
}) {
  const { c, radius, space, type } = useTheme();
  const scale = useSharedValue(1);
  const animated = useAnimatedStyle(() => ({ transform: [{ scale: scale.value }] }));
  const palette: Record<ButtonKind, { bg: string; fg: string; border: string }> = {
    primary: { bg: c.accent, fg: c.accentText, border: c.accent },
    secondary: { bg: c.surface, fg: c.text, border: c.border },
    ghost: { bg: 'transparent', fg: c.text, border: 'transparent' },
    danger: { bg: c.danger, fg: '#fff', border: c.danger },
  };
  const p = palette[kind];
  const inactive = disabled || loading;
  return (
    <Animated.View style={[animated, style]}>
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ disabled: !!inactive, busy: !!loading }}
        disabled={inactive}
        onPressIn={() => (scale.value = withSpring(0.97, motion.spring))}
        onPressOut={() => (scale.value = withSpring(1, motion.spring))}
        style={{
          minHeight: 52,
          borderRadius: radius.pill,
          backgroundColor: p.bg,
          borderWidth: 1,
          borderColor: p.border,
          paddingHorizontal: space.xl,
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'center',
          gap: space.sm,
          opacity: disabled ? 0.45 : 1,
        }}
        {...rest}
      >
        {loading ? <ActivityIndicator color={p.fg} /> : icon}
        <RNText style={[type.bodyStrong, { color: p.fg }]}>{title}</RNText>
      </Pressable>
    </Animated.View>
  );
}

export function Field({
  label,
  error,
  prefix,
  style,
  ref,
  ...rest
}: TextInputProps & { label?: string; error?: string | null; prefix?: string; ref?: Ref<ComponentRef<typeof TextInput>> }) {
  {
    const { c, radius, space, type } = useTheme();
    return (
      <View style={{ gap: space.xs }}>
        {label ? <Text variant="label" tone="muted">{label}</Text> : null}
        <View
          style={{
            flexDirection: 'row',
            alignItems: 'center',
            backgroundColor: c.surface,
            borderRadius: radius.md,
            borderWidth: 1.5,
            borderColor: error ? c.danger : c.border,
            paddingHorizontal: space.lg,
            minHeight: 54,
          }}
        >
          {prefix ? <Text variant="bodyStrong" style={{ marginRight: space.sm }}>{prefix}</Text> : null}
          <TextInput
            ref={ref}
            placeholderTextColor={c.textMuted}
            style={[type.body, { flex: 1, color: c.text, paddingVertical: space.md } as TextStyle, style]}
            {...rest}
          />
        </View>
        {error ? <Text variant="caption" tone="danger">{error}</Text> : null}
      </View>
    );
  }
}

export function Chip({
  label,
  selected,
  onPress,
}: {
  label: string;
  selected?: boolean;
  onPress?: () => void;
}) {
  const { c, radius, space } = useTheme();
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected: !!selected }}
      onPress={onPress}
      style={{
        paddingHorizontal: space.md,
        paddingVertical: space.sm,
        borderRadius: radius.pill,
        backgroundColor: selected ? c.text : c.surface,
        borderWidth: 1,
        borderColor: selected ? c.text : c.border,
      }}
    >
      <Text variant="label" style={{ color: selected ? c.bg : c.text }}>{label}</Text>
    </Pressable>
  );
}

export function Badge({ label, tone = 'neutral' }: { label: string; tone?: 'neutral' | 'accent' | 'success' | 'warning' | 'danger' }) {
  const { c, radius, space } = useTheme();
  const bg = { neutral: c.surfaceAlt, accent: c.accent, success: c.success, warning: c.warning, danger: c.danger }[tone];
  const fg = tone === 'neutral' ? c.text : tone === 'accent' ? c.accentText : '#fff';
  return (
    <View style={{ alignSelf: 'flex-start', backgroundColor: bg, borderRadius: radius.pill, paddingHorizontal: space.sm, paddingVertical: 2 }}>
      <Text variant="caption" style={{ color: fg, fontFamily: 'Inter_600SemiBold' }}>{label}</Text>
    </View>
  );
}

export function Divider() {
  const { c } = useTheme();
  return <View style={{ height: 1, backgroundColor: c.border }} />;
}
