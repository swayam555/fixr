import type { ExpoConfig } from 'expo/config';

const variant = process.env.APP_VARIANT ?? 'development';
const suffix = variant === 'production' ? '' : `.${variant}`;

const config: ExpoConfig = {
  name: variant === 'production' ? 'Fixr' : `Fixr (${variant})`,
  slug: 'fixr-customer',
  scheme: 'fixr',
  version: '0.1.0',
  orientation: 'portrait',
  icon: './assets/icon.png',
  userInterfaceStyle: 'automatic',
  ios: {
    bundleIdentifier: `in.fixr.customer${suffix}`,
    supportsTablet: false,
    infoPlist: {
      NSCameraUsageDescription: 'Fixr uses the camera so you can photograph the problem and get the right pro.',
      NSMicrophoneUsageDescription: 'Fixr records a short voice note to understand the job.',
      NSLocationWhenInUseUsageDescription: 'Fixr uses your location to find serviceable addresses and nearby pros.',
    },
  },
  android: {
    package: `in.fixr.customer${suffix.replace(/-/g, '_')}`,
    adaptiveIcon: {
      backgroundColor: '#111316',
      foregroundImage: './assets/android-icon-foreground.png',
      backgroundImage: './assets/android-icon-background.png',
      monochromeImage: './assets/android-icon-monochrome.png',
    },
    permissions: ['CAMERA', 'RECORD_AUDIO', 'ACCESS_FINE_LOCATION', 'POST_NOTIFICATIONS'],
  },
  plugins: ['expo-router', 'expo-secure-store', 'expo-font', '../../plugins/with-gradle-tuning'],
  experiments: { typedRoutes: true },
  extra: {
    apiUrl: process.env.EXPO_PUBLIC_API_URL,
    eas: { projectId: process.env.EAS_PROJECT_ID_CUSTOMER },
  },
};

export default config;
