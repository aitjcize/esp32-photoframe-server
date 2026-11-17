// The server refuses to remove what a frame is bound to (a connection, a
// gallery album, a topic) with a 409 whose body names the frames:
// {code: 'in_use', devices: [{id, name}]}. The frame would otherwise move to
// a wider pool than its owner chose. inUseMessage turns that answer into the
// text for a dialog, or returns null for any other error.
export const inUseMessage = (
  e: any,
  what: string,
  hint = 'Change their Image Source under Settings → Devices first, then try again.'
): string | null => {
  const data = e?.response?.data;
  if (e?.response?.status !== 409 || data?.code !== 'in_use') return null;
  const names: string[] = (data.devices || []).map((d: any) => d.name);
  const who =
    names.length === 1
      ? `the frame ${names[0]}`
      : `${names.length} frames: ${names.join(', ')}`;
  return `${what} is used by ${who}. ${hint}`;
};
