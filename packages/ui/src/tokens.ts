export const palette = {
  ink900: '#0E1013',
  ink800: '#16191E',
  ink700: '#20242B',
  ink600: '#2D323A',
  ink400: '#5B626D',
  ink300: '#8A919C',
  ink200: '#C3C8CF',
  ink100: '#E4E6E9',
  paper: '#F5F3EE',
  paperRaised: '#FFFFFF',
  hivis: '#D4F24A',
  hivisDeep: '#A9C61F',
  hivisInk: '#1B2200',
  signal: '#3E7BFA',
  success: '#1E9E5A',
  warning: '#F2A93B',
  danger: '#E5484D',
} as const;

export type ColorScheme = 'light' | 'dark';

export interface ThemeColors {
  bg: string;
  surface: string;
  surfaceAlt: string;
  border: string;
  text: string;
  textMuted: string;
  textInverse: string;
  accent: string;
  accentText: string;
  signal: string;
  success: string;
  warning: string;
  danger: string;
  mapRoute: string;
}

export const colors: Record<ColorScheme, ThemeColors> = {
  light: {
    bg: palette.paper,
    surface: palette.paperRaised,
    surfaceAlt: '#ECE9E2',
    border: '#DDD9D0',
    text: palette.ink900,
    textMuted: palette.ink400,
    textInverse: palette.paper,
    accent: palette.hivis,
    accentText: palette.hivisInk,
    signal: palette.signal,
    success: palette.success,
    warning: palette.warning,
    danger: palette.danger,
    mapRoute: palette.ink900,
  },
  dark: {
    bg: palette.ink900,
    surface: palette.ink800,
    surfaceAlt: palette.ink700,
    border: palette.ink600,
    text: '#F2F3F5',
    textMuted: palette.ink300,
    textInverse: palette.ink900,
    accent: palette.hivis,
    accentText: palette.hivisInk,
    signal: '#6E9BFF',
    success: '#3CC27D',
    warning: palette.warning,
    danger: '#FF6B6F',
    mapRoute: palette.hivis,
  },
};

export const space = { xxs: 2, xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 32, xxxl: 48 } as const;
export const radius = { sm: 8, md: 12, lg: 18, xl: 28, pill: 999 } as const;

export const fonts = {
  display: 'Sora_700Bold',
  displayMedium: 'Sora_600SemiBold',
  body: 'Inter_400Regular',
  bodyMedium: 'Inter_500Medium',
  bodySemibold: 'Inter_600SemiBold',
} as const;

export const type = {
  hero: { fontFamily: fonts.display, fontSize: 32, lineHeight: 38, letterSpacing: -0.6 },
  title: { fontFamily: fonts.display, fontSize: 22, lineHeight: 28, letterSpacing: -0.3 },
  heading: { fontFamily: fonts.displayMedium, fontSize: 17, lineHeight: 22 },
  body: { fontFamily: fonts.body, fontSize: 15, lineHeight: 22 },
  bodyStrong: { fontFamily: fonts.bodySemibold, fontSize: 15, lineHeight: 22 },
  label: { fontFamily: fonts.bodyMedium, fontSize: 13, lineHeight: 18 },
  caption: { fontFamily: fonts.body, fontSize: 12, lineHeight: 16 },
  mono: { fontFamily: fonts.bodySemibold, fontSize: 28, lineHeight: 34, letterSpacing: 6 },
} as const;

export type TypeVariant = keyof typeof type;

export const motion = {
  fast: 140,
  base: 220,
  slow: 360,
  spring: { damping: 18, stiffness: 220, mass: 0.9 },
} as const;
