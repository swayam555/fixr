import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { useColorScheme } from 'react-native';
import { colors, radius, space, type, type ColorScheme, type ThemeColors } from './tokens';

export interface Theme {
  scheme: ColorScheme;
  c: ThemeColors;
  space: typeof space;
  radius: typeof radius;
  type: typeof type;
}

const ThemeContext = createContext<Theme | null>(null);

export function ThemeProvider({ children, scheme }: { children: ReactNode; scheme?: ColorScheme }) {
  const system = useColorScheme();
  const resolved: ColorScheme = scheme ?? (system === 'dark' ? 'dark' : 'light');
  const value = useMemo<Theme>(
    () => ({ scheme: resolved, c: colors[resolved], space, radius, type }),
    [resolved],
  );
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): Theme {
  const theme = useContext(ThemeContext);
  if (!theme) throw new Error('useTheme must be used inside <ThemeProvider>');
  return theme;
}
