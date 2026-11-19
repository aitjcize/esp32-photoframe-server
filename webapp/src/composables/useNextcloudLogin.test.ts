import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const mocks = vi.hoisted(() => ({
  nextcloudLoginStart: vi.fn(),
  nextcloudLoginStatus: vi.fn(),
  test: vi.fn(),
}));
vi.mock('../stores/connections', () => ({
  useConnectionsStore: () => mocks,
}));
vi.mock('vue', async (orig) => ({
  ...(await orig<typeof import('vue')>()),
  onBeforeUnmount: () => {},
}));

import { useNextcloudLogin } from './useNextcloudLogin';

describe('useNextcloudLogin', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mocks.nextcloudLoginStart.mockReset().mockResolvedValue('https://nc/flow');
    mocks.nextcloudLoginStatus.mockReset();
    mocks.test.mockReset().mockResolvedValue({ id: 1, connected: true });
  });
  afterEach(() => vi.useRealTimers());

  it('sends the tab to the sign-in page and resolves once access is granted', async () => {
    mocks.nextcloudLoginStatus
      .mockResolvedValueOnce({ state: 'pending' })
      .mockResolvedValueOnce({ state: 'done' });
    const tab = { closed: false, location: { href: '' }, close: vi.fn() };
    const nc = useNextcloudLogin();

    const p = nc.signIn(1, tab as any);
    await vi.advanceTimersByTimeAsync(0);
    expect(tab.location.href).toBe('https://nc/flow');
    expect(nc.loginUrl.value).toBe('https://nc/flow');
    expect(nc.waiting.value).toBe(true);

    await vi.advanceTimersByTimeAsync(4000);
    await expect(p).resolves.toEqual({ id: 1, connected: true });
    expect(mocks.test).toHaveBeenCalledWith(1);
    expect(nc.waiting.value).toBe(false);
    expect(nc.loginUrl.value).toBe('');
  });

  it('rejects with the server reason when the sign-in failed', async () => {
    mocks.nextcloudLoginStatus.mockResolvedValue({
      state: 'failed',
      error: 'the sign-in expired',
    });
    const nc = useNextcloudLogin();
    const p = nc.signIn(1, null);
    const assertion = expect(p).rejects.toThrow('the sign-in expired');
    await vi.advanceTimersByTimeAsync(2000);
    await assertion;
    expect(mocks.test).not.toHaveBeenCalled();
  });

  it('resolves null when cancelled while waiting', async () => {
    mocks.nextcloudLoginStatus.mockResolvedValue({ state: 'pending' });
    const nc = useNextcloudLogin();
    const p = nc.signIn(1, null);
    await vi.advanceTimersByTimeAsync(2000);
    nc.cancel();
    expect(nc.waiting.value).toBe(false);
    await vi.advanceTimersByTimeAsync(2000);
    await expect(p).resolves.toBeNull();
  });

  it('closes the tab when the flow cannot start', async () => {
    mocks.nextcloudLoginStart.mockRejectedValue(new Error('not nextcloud'));
    const tab = { closed: false, location: { href: '' }, close: vi.fn() };
    const nc = useNextcloudLogin();
    await expect(nc.signIn(1, tab as any)).rejects.toThrow('not nextcloud');
    expect(tab.close).toHaveBeenCalled();
    expect(nc.waiting.value).toBe(false);
  });
});
