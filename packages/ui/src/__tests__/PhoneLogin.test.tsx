import { fireEvent, render, screen, waitFor } from '@testing-library/react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { PhoneLogin } from '../PhoneLogin';
import { ThemeProvider } from '../theme';

const copy = { eyebrow: 'E', title: 'T', subtitle: 'S' };
const normalise = (raw: string) => (/^[6-9]\d{9}$/.test(raw.replace(/\D/g, '')) ? `+91${raw.replace(/\D/g, '')}` : null);

async function renderLogin(overrides: Partial<React.ComponentProps<typeof PhoneLogin>> = {}) {
  const props = {
    copy,
    validatePhone: normalise,
    requestOtp: jest.fn(async () => ({ challengeId: 'c1', resendInSec: 30, devOtp: '123456' })),
    verifyOtp: jest.fn(async () => {}),
    ...overrides,
  };
  await render(
    <SafeAreaProvider initialMetrics={{ frame: { x: 0, y: 0, width: 390, height: 844 }, insets: { top: 0, left: 0, right: 0, bottom: 0 } }}>
      <ThemeProvider scheme="light">
        <PhoneLogin {...props} />
      </ThemeProvider>
    </SafeAreaProvider>,
  );
  return props;
}

it('rejects an invalid number without calling the API', async () => {
  const props = await renderLogin();
  await fireEvent.changeText(screen.getByTestId('phone-input'), '12345');
  await fireEvent.press(screen.getByTestId('send-otp'));
  expect(await screen.findByText('Enter a 10-digit Indian mobile number')).toBeTruthy();
  expect(props.requestOtp).not.toHaveBeenCalled();
});

it('sends the normalised number, then auto-verifies on the sixth digit', async () => {
  const props = await renderLogin();
  await fireEvent.changeText(screen.getByTestId('phone-input'), '98765 43210');
  await fireEvent.press(screen.getByTestId('send-otp'));
  await waitFor(() => expect(props.requestOtp).toHaveBeenCalledWith('+919876543210'));

  await fireEvent.changeText(await screen.findByTestId('otp-input'), '123456');
  await waitFor(() => expect(props.verifyOtp).toHaveBeenCalledWith('c1', '123456'));
});

it('shows the server error and clears the code when verification fails', async () => {
  await renderLogin({ verifyOtp: jest.fn(async () => Promise.reject(new Error('That code is not right'))) });
  await fireEvent.changeText(screen.getByTestId('phone-input'), '9876543210');
  await fireEvent.press(screen.getByTestId('send-otp'));
  await fireEvent.changeText(await screen.findByTestId('otp-input'), '000000');
  expect(await screen.findByText('That code is not right')).toBeTruthy();
});
