import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Device, StoredFramePassword } from '../api';

const mocks = vi.hoisted(() => ({
  setDeviceHttpPassword: vi.fn(),
  showMessage: vi.fn(),
}));
vi.mock('../api', () => ({
  setDeviceHttpPassword: mocks.setDeviceHttpPassword,
}));
vi.mock('./useSnackbar', () => ({
  useSnackbar: () => ({ showMessage: mocks.showMessage }),
}));

import { useFramePasswordPrompt } from './useFramePasswordPrompt';

// Lets every pending microtask and macrotask run.
const flush = () => new Promise((r) => setTimeout(r, 0));

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const stored = (
  over: Partial<StoredFramePassword> = {}
): StoredFramePassword => ({
  http_password_set: true,
  verified: true,
  auth_required: false,
  auth_failed_at: null,
  ...over,
});

let nextId = 100;
const refusing = (name: string): Device =>
  ({
    id: nextId++,
    name,
    host: `${name.toLowerCase()}.local`,
    auth_required: true,
    auth_failed_at: '2026-09-26T10:00:00Z',
    http_password_set: true,
  }) as Device;

const settled = async (p: Promise<unknown>) => {
  let done = false;
  void p.then(() => (done = true));
  await flush();
  return done;
};

describe('useFramePasswordPrompt', () => {
  beforeEach(() => {
    mocks.setDeviceHttpPassword.mockReset();
    mocks.showMessage.mockReset();
  });

  it('settles the prompt a save was made from, not one asked for meanwhile', async () => {
    const { state, promptFramePassword, save } = useFramePasswordPrompt();
    const a = refusing('A');
    const b = refusing('B');
    const request = deferred<StoredFramePassword>();
    mocks.setDeviceHttpPassword.mockReturnValueOnce(request.promise);

    const resultA = promptFramePassword(a);
    await flush();
    expect(state.device?.id).toBe(a.id);
    state.password = 'pw-a';
    const saving = save();
    await flush();
    expect(state.saving).toBe(true);

    // B asks while A's save is out: it waits, and A stays up.
    const resultB = promptFramePassword(b);
    await flush();
    expect(state.device?.id).toBe(a.id);
    expect(await settled(resultA)).toBe(false);

    request.resolve(stored());
    await saving;
    await expect(resultA).resolves.toEqual({ saved: true, verified: true });
    expect(mocks.setDeviceHttpPassword).toHaveBeenCalledWith(a.id, 'pw-a');
    expect(a.auth_required).toBe(false);

    // Now B has its turn, unanswered, with its own device untouched.
    await flush();
    expect(state.show).toBe(true);
    expect(state.device?.id).toBe(b.id);
    expect(state.password).toBe('');
    expect(await settled(resultB)).toBe(false);
    expect(b.auth_required).toBe(true);
  });

  it('keeps a rejected password in place, open for another try', async () => {
    const { state, promptFramePassword, save } = useFramePasswordPrompt();
    const d = refusing('C');
    mocks.setDeviceHttpPassword.mockRejectedValueOnce({
      response: {
        status: 409,
        data: { code: 'frame_auth_required', error: 'The frame rejected it.' },
      },
    });

    const result = promptFramePassword(d);
    await flush();
    state.password = 'wrong';
    await save();
    expect(state.show).toBe(true);
    expect(state.error).toBe('The frame rejected it.');
    expect(state.saving).toBe(false);
    expect(await settled(result)).toBe(false);
    expect(d.auth_required).toBe(true);

    mocks.setDeviceHttpPassword.mockResolvedValueOnce(
      stored({ verified: false, warning: 'Asleep.' })
    );
    state.password = 'right';
    await save();
    await expect(result).resolves.toEqual({ saved: true, verified: false });
    expect(mocks.showMessage).toHaveBeenLastCalledWith('Asleep.');
    expect(state.show).toBe(false);
    // The password does not outlive the prompt.
    expect(state.password).toBe('');
    expect(state.error).toBe('');
  });

  it('does not send an empty or over-long password', async () => {
    const { state, promptFramePassword, save } = useFramePasswordPrompt();
    const d = refusing('D');
    void promptFramePassword(d);
    await flush();

    await save();
    expect(state.error).toMatch(/Enter the frame's password/);
    state.password = 'x'.repeat(64);
    await save();
    expect(state.error).toMatch(/63 bytes/);
    expect(mocks.setDeviceHttpPassword).not.toHaveBeenCalled();
    expect(state.show).toBe(true);
  });

  it('reset closes the prompt and forgets what it held', async () => {
    const { state, promptFramePassword, promptIfNeeded, dismiss, reset } =
      useFramePasswordPrompt();
    const d = refusing('F');
    const result = promptFramePassword(d);
    await flush();
    state.password = 'typed';

    reset();
    await expect(result).resolves.toEqual({ saved: false });
    expect(state.show).toBe(false);
    expect(state.device).toBeNull();
    expect(state.password).toBe('');

    // The "Not now"s go too: the next session asks afresh.
    void promptFramePassword(d);
    await flush();
    dismiss();
    reset();
    promptIfNeeded([d]);
    await flush();
    expect(state.show).toBe(true);
    expect(state.device?.id).toBe(d.id);
    reset();
  });

  it('remembers "Not now" for that run of refusals', async () => {
    const { state, promptFramePassword, promptIfNeeded, dismiss } =
      useFramePasswordPrompt();
    const d = refusing('E');
    const result = promptFramePassword(d);
    await flush();
    state.password = 'half-typed';
    dismiss();
    await expect(result).resolves.toEqual({ saved: false });
    expect(state.show).toBe(false);
    expect(state.password).toBe('');

    promptIfNeeded([d]);
    await flush();
    expect(state.show).toBe(false);

    // A new run of refusals asks again.
    d.auth_failed_at = '2026-09-27T08:00:00Z';
    promptIfNeeded([d]);
    await flush();
    expect(state.show).toBe(true);
    expect(state.device?.id).toBe(d.id);
    dismiss();
  });
});
