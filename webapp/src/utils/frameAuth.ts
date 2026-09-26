import type { Device, StoredFramePassword } from '../api';

// The server answers a frame's 401 as a 409 carrying this code, because the
// webapp reads a 401 as its own session expiring (see api.ts). Other 409s
// (a host edited elsewhere, say) do not carry it.
export const FRAME_AUTH_REQUIRED_CODE = 'frame_auth_required';

// isFrameAuthRefusal reports whether an API error says the frame refused
// the password the server has for it, so the user should be asked for it.
export function isFrameAuthRefusal(e: unknown): boolean {
  const err = e as
    | { response?: { status?: number; data?: { code?: string } } }
    | undefined;
  return (
    err?.response?.status === 409 &&
    err.response.data?.code === FRAME_AUTH_REQUIRED_CODE
  );
}

export type Refusal = Pick<Device, 'id' | 'auth_required' | 'auth_failed_at'>;

// refusalKey identifies a run of refusals by when it started: auth_failed_at
// stays put while the frame keeps refusing, and changes only once the frame
// accepted something and then refused again. "Not now" is remembered for a
// run, so a new one asks again.
export function refusalKey(device: Refusal): string {
  return device.auth_failed_at || '';
}

// needsPasswordPrompt: the device's frame is refusing the server, and the
// user has not put off this run of refusals.
export function needsPasswordPrompt(
  device: Refusal,
  dismissed: ReadonlyMap<number, string>
): boolean {
  return (
    !!device.auth_required && dismissed.get(device.id) !== refusalKey(device)
  );
}

// firstNeedingPassword picks the device to ask about when a list loads: the
// first one whose frame is refusing and which the user has not put off.
export function firstNeedingPassword<D extends Refusal>(
  devices: readonly D[],
  dismissed: ReadonlyMap<number, string>
): D | undefined {
  return devices.find((d) => needsPasswordPrompt(d, dismissed));
}

// framePasswordPromptText is what the prompt says about the device. The
// body differs by whether the server has a password at all: a wrong one and
// none are different things to fix on the frame's side.
export function framePasswordPromptText(
  device: Pick<Device, 'name' | 'host' | 'http_password_set'>
): { title: string; body: string } {
  const what = device.http_password_set
    ? 'The frame refuses the password this server has for it.'
    : 'The frame asks for a password, and this server has none for it.';
  return {
    title: `${device.name || device.host} now requires a password`,
    body: `${what} Enter the password set on the frame so the server can reach it again. This does not change the frame.`,
  };
}

// passwordStoredMessage is the one-line outcome of storing a password.
export function passwordStoredMessage(
  res: Pick<StoredFramePassword, 'verified' | 'warning'>,
  deviceName: string
): string {
  if (res.verified) {
    return `${deviceName} accepted the password.`;
  }
  return (
    res.warning ||
    'The password was stored. The frame could not be reached, so it will be checked on its next wake.'
  );
}
