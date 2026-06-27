// Case-insensitive substring match used to filter album lists by a search box.
export const matchesAlbum = (name: string, q: string) =>
  !q || (name || '').toLowerCase().includes(q.toLowerCase());

// Sort checked albums to the top, then alphabetically — so selected albums are
// grouped and easy to find/uncheck.
export const sortCheckedFirst =
  (key: string, isChecked: (a: any) => boolean) => (a: any, b: any) => {
    const ca = isChecked(a) ? 1 : 0;
    const cb = isChecked(b) ? 1 : 0;
    if (ca !== cb) return cb - ca;
    return (a[key] || '').localeCompare(b[key] || '', undefined, {
      sensitivity: 'base',
    });
  };
