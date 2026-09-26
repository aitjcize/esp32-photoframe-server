import { reactive } from 'vue';
import { setDeviceHttpPassword, type Device } from '../api';
import { useSnackbar } from './useSnackbar';
import { getApiError } from '../utils/errors';
import {
  firstNeedingPassword,
  passwordStoredMessage,
  refusalKey,
} from '../utils/frameAuth';
import {
  FRAME_PASSWORD_MAX_BYTES,
  framePasswordBytes,
} from '../utils/framePassword';

// One prompt for the whole app -- FramePasswordPrompt.vue, mounted once in
// App.vue like the snackbar -- so the device list, the device dialog and the
// gallery's push all ask the same way, and none owns a dialog of its own.
// Module-level state, as in useSnackbar.
const state = reactive({
  show: false,
  device: null as Device | null,
  password: '',
  error: '',
  saving: false,
});

// "Not now", per device, for the run of refusals it was said to (see
// refusalKey). In memory only: a reload asks again, and the badge stays
// meanwhile.
const dismissed = new Map<number, string>();

export type FramePasswordPromptResult =
  | { saved: true; verified: boolean }
  | { saved: false };

let resolveOpen: ((r: FramePasswordPromptResult) => void) | null = null;

// The save in flight, if any. A prompt asked for meanwhile waits for it, so
// a save always settles the prompt it was made from, never one that took
// its place while the request was out.
let inFlight: Promise<void> | null = null;

function finish(result: FramePasswordPromptResult) {
  state.show = false;
  // Not kept for the rest of the session: the next prompt starts blank.
  state.password = '';
  state.error = '';
  const resolve = resolveOpen;
  resolveOpen = null;
  resolve?.(result);
}

export function useFramePasswordPrompt() {
  const { showMessage } = useSnackbar();

  // promptFramePassword asks for device's frame password and resolves once
  // the prompt closes: saved, with whether the frame confirmed the password,
  // or not. The device object is updated in place with what the server then
  // reports (auth_required, http_password_set), so the list item it came
  // from reads right without a reload.
  async function promptFramePassword(
    device: Device
  ): Promise<FramePasswordPromptResult> {
    while (inFlight) await inFlight;
    if (state.show) {
      // Asked about another device meanwhile: that prompt is over.
      finish({ saved: false });
    }
    state.device = device;
    state.password = '';
    state.error = '';
    state.saving = false;
    state.show = true;
    return new Promise((resolve) => {
      resolveOpen = resolve;
    });
  }

  // promptIfNeeded is the unasked-for prompt when a device list loads: for
  // the first device whose frame is refusing the server and which the user
  // has not put off. Nothing happens while a prompt is already open.
  function promptIfNeeded(devices: readonly Device[]) {
    if (state.show) return;
    const device = firstNeedingPassword(devices, dismissed);
    if (device) void promptFramePassword(device);
  }

  function dismiss() {
    if (state.device) dismissed.set(state.device.id, refusalKey(state.device));
    finish({ saved: false });
  }

  // reset closes the prompt and drops everything it held -- the device, a
  // typed password, the "Not now"s -- for a logout. The state outlives the
  // components, so merely unmounting the prompt would show it again, as it
  // was, at the next login.
  function reset() {
    finish({ saved: false });
    state.device = null;
    state.password = '';
    state.error = '';
    state.saving = false;
    dismissed.clear();
  }

  async function save() {
    const device = state.device;
    if (!device || inFlight) return;
    if (state.password === '') {
      state.error = "Enter the frame's password.";
      return;
    }
    if (framePasswordBytes(state.password) > FRAME_PASSWORD_MAX_BYTES) {
      state.error = `The frame accepts at most ${FRAME_PASSWORD_MAX_BYTES} bytes of password.`;
      return;
    }
    state.error = '';
    state.saving = true;
    inFlight = (async () => {
      try {
        const res = await setDeviceHttpPassword(device.id, state.password);
        device.http_password_set = res.http_password_set;
        device.auth_required = res.auth_required;
        device.auth_failed_at = res.auth_failed_at;
        showMessage(passwordStoredMessage(res, device.name || device.host));
        finish({ saved: true, verified: res.verified });
      } catch (e: unknown) {
        // A rejected password (409) and anything else alike: shown in
        // place, with the prompt kept open to try again.
        state.error = getApiError(e, 'Could not store the password.');
      } finally {
        state.saving = false;
      }
    })();
    try {
      await inFlight;
    } finally {
      inFlight = null;
    }
  }

  return { state, promptFramePassword, promptIfNeeded, dismiss, save, reset };
}
