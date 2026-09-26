import { describe, it, expect } from 'vitest';
import {
  FRAME_AUTH_REQUIRED_CODE,
  firstNeedingPassword,
  framePasswordPromptText,
  isFrameAuthRefusal,
  needsPasswordPrompt,
  passwordStoredMessage,
  refusalKey,
} from './frameAuth';

const refusal = (status: number, code?: string) => ({
  response: { status, data: code ? { code, error: 'x' } : { error: 'x' } },
});

describe('isFrameAuthRefusal', () => {
  it('recognises the 409 that stands for the frame 401', () => {
    expect(isFrameAuthRefusal(refusal(409, FRAME_AUTH_REQUIRED_CODE))).toBe(
      true
    );
  });

  it('leaves other conflicts and statuses alone', () => {
    expect(isFrameAuthRefusal(refusal(409))).toBe(false);
    expect(isFrameAuthRefusal(refusal(409, 'something_else'))).toBe(false);
    expect(isFrameAuthRefusal(refusal(401, FRAME_AUTH_REQUIRED_CODE))).toBe(
      false
    );
    expect(isFrameAuthRefusal(new Error('network'))).toBe(false);
    expect(isFrameAuthRefusal(undefined)).toBe(false);
  });
});

describe('needsPasswordPrompt', () => {
  const since = '2026-09-26T10:00:00Z';
  const device = { id: 7, auth_required: true, auth_failed_at: since };

  it('asks for a refusing device the user has not put off', () => {
    expect(needsPasswordPrompt(device, new Map())).toBe(true);
  });

  it('does not ask for a device whose frame is fine', () => {
    expect(
      needsPasswordPrompt({ id: 7, auth_required: false }, new Map())
    ).toBe(false);
  });

  it('stays quiet for the run of refusals the user put off', () => {
    const dismissed = new Map([[7, refusalKey(device)]]);
    expect(needsPasswordPrompt(device, dismissed)).toBe(false);
  });

  it('asks again for a new run of refusals', () => {
    const dismissed = new Map([[7, refusalKey(device)]]);
    const later = { ...device, auth_failed_at: '2026-09-27T08:00:00Z' };
    expect(needsPasswordPrompt(later, dismissed)).toBe(true);
  });

  it('keys the dismissal per device', () => {
    const dismissed = new Map([[8, refusalKey(device)]]);
    expect(needsPasswordPrompt(device, dismissed)).toBe(true);
  });

  it('treats a missing auth_failed_at as one run', () => {
    const undated = { id: 7, auth_required: true, auth_failed_at: null };
    expect(needsPasswordPrompt(undated, new Map())).toBe(true);
    expect(
      needsPasswordPrompt(undated, new Map([[7, refusalKey(undated)]]))
    ).toBe(false);
  });
});

describe('firstNeedingPassword', () => {
  it('picks the first refusing device not put off', () => {
    const devices = [
      { id: 1, auth_required: false },
      { id: 2, auth_required: true, auth_failed_at: 'a' },
      { id: 3, auth_required: true, auth_failed_at: 'b' },
    ];
    expect(firstNeedingPassword(devices, new Map())?.id).toBe(2);
    expect(firstNeedingPassword(devices, new Map([[2, 'a']]))?.id).toBe(3);
    expect(
      firstNeedingPassword(
        devices,
        new Map([
          [2, 'a'],
          [3, 'b'],
        ])
      )
    ).toBeUndefined();
  });
});

describe('framePasswordPromptText', () => {
  it('names the device and says whether a password is stored', () => {
    const wrong = framePasswordPromptText({
      name: 'Kitchen',
      host: 'kitchen.local',
      http_password_set: true,
    });
    expect(wrong.title).toBe('Kitchen now requires a password');
    expect(wrong.body).toMatch(/refuses the password this server has/);

    const none = framePasswordPromptText({
      name: '',
      host: 'kitchen.local',
      http_password_set: false,
    });
    expect(none.title).toBe('kitchen.local now requires a password');
    expect(none.body).toMatch(/has none for it/);
  });
});

describe('passwordStoredMessage', () => {
  it('reports a confirmed password', () => {
    expect(passwordStoredMessage({ verified: true }, 'Kitchen')).toBe(
      'Kitchen accepted the password.'
    );
  });

  it("passes the server's reason through when unverified", () => {
    expect(
      passwordStoredMessage({ verified: false, warning: 'Asleep.' }, 'Kitchen')
    ).toBe('Asleep.');
    expect(passwordStoredMessage({ verified: false }, 'Kitchen')).toMatch(
      /next wake/
    );
  });
});
