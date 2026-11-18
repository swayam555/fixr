import type { ExpoConfig } from 'expo/config';

const variant = process.env.APP_VARIANT ?? 'development';
const suffix = variant === 'production' ? '' : `.${variant}`;

const config: ExpoConfig = {
  name: variant === 'production' ? 'Fixr Partner' : `Fixr Partner (${variant})`,
  slug: 'fixr-partner',
  scheme: 'fixrpartner',
  version: '0.1.0',
  orientation: 'portrait',
  icon: './assets/icon.png',
  userInterfaceStyle: 'automatic',
  ios: {
    bundleIdentifier: `in.fixr.partner${suffix}`,
    supportsTablet: false,
    infoPlist: {
      NSCameraUsageDescription: 'Fixr Partner uses the camera for KYC documents and job photos.',
      NSLocationWhenInUseUsageDescription: 'Fixr Partner uses your location to send you nearby jobs.',
      NSLocationAlwaysAndWhenInUseUsageDescription:
        'While you are online, Fixr Partner shares your location in the background so customers can track their pro and you keep receiving nearby jobs.',
      NSMotionUsageDescription: 'Fixr Partner detects when you are stationary to save battery.',
      UIBackgroundModes: ['location', 'fetch', 'remote-notification'],
    },
  },
  android: {
    package: `in.fixr.partner${suffix.replace(/-/g, '_')}`,
    adaptiveIcon: {
      backgroundColor: '#D4F24A',
      foregroundImage: './assets/android-icon-foreground.png',
      backgroundImage: './assets/android-icon-background.png',
      monochromeImage: './assets/android-icon-monochrome.png',
    },
    permissions: [
      'CAMERA',
      'ACCESS_FINE_LOCATION',
      'ACCESS_COARSE_LOCATION',
      'ACCESS_BACKGROUND_LOCATION',
      'FOREGROUND_SERVICE',
      'FOREGROUND_SERVICE_LOCATION',
      'ACTIVITY_RECOGNITION',
      'POST_NOTIFICATIONS',
      'USE_FULL_SCREEN_INTENT',
      'WAKE_LOCK',
    ],
  },
  plugins: ['expo-router', 'expo-secure-store', 'expo-font', '../../plugins/with-gradle-tuning'],
  experiments: { typedRoutes: true },
  extra: {
    apiUrl: process.env.EXPO_PUBLIC_API_URL,
    eas: { projectId: process.env.EAS_PROJECT_ID_PARTNER },
  },
};

export default config;
