import Constants from 'expo-constants';
import { Platform } from 'react-native';

const fromConfig = (Constants.expoConfig?.extra as { apiUrl?: string } | undefined)?.apiUrl;
const devHost = Platform.OS === 'android' ? 'http://10.0.2.2:8080' : 'http://localhost:8080';

export const API_URL = process.env.EXPO_PUBLIC_API_URL ?? fromConfig ?? devHost;
export const WS_URL = API_URL.replace(/^http/, 'ws') + '/ws';
