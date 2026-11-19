import { onBeforeUnmount, ref } from 'vue';
import { useConnectionsStore, type Connection } from '../stores/connections';

// How often the dashboard asks the server whether the user granted access,
// and how long it waits: Nextcloud expires a login flow after 20 minutes.
const POLL_MS = 2000;
const DEADLINE_MS = 20 * 60 * 1000;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

// Signs a Nextcloud Memories connection in through Nextcloud's Login Flow v2:
// the server starts the flow, the user signs in and grants access on their
// Nextcloud in a new tab, and the server picks up the app password Nextcloud
// issues. The dashboard never sees the user name or password.
//
// Browsers only allow a new tab straight from a click, before anything is
// awaited: call openTab() first thing in the click handler and pass its
// result to signIn(). When the tab was blocked, loginUrl offers the page as a
// link instead.
export function useNextcloudLogin() {
  const store = useConnectionsStore();
  const waiting = ref(false);
  const loginUrl = ref('');
  let generation = 0;

  const openTab = (): Window | null => {
    const tab = window.open('', '_blank');
    // The page is Nextcloud's: it gets no handle on the dashboard.
    if (tab) tab.opener = null;
    return tab;
  };

  // Resolves with the verified connection once access is granted, with null
  // when cancel() was called, and rejects when the sign-in failed.
  const signIn = async (
    id: number,
    tab: Window | null
  ): Promise<Connection | null> => {
    const mine = ++generation;
    waiting.value = true;
    try {
      let url: string;
      try {
        url = await store.nextcloudLoginStart(id);
      } catch (e) {
        tab?.close();
        throw e;
      }
      if (mine !== generation) {
        tab?.close();
        return null;
      }
      loginUrl.value = url;
      if (tab && !tab.closed) tab.location.href = url;

      const deadline = Date.now() + DEADLINE_MS;
      while (Date.now() < deadline) {
        await sleep(POLL_MS);
        if (mine !== generation) return null;
        const status = await store.nextcloudLoginStatus(id);
        if (mine !== generation) return null;
        if (status.state === 'done') return await store.test(id);
        if (status.state === 'failed') {
          throw new Error(status.error || 'The sign-in failed.');
        }
      }
      throw new Error('The sign-in expired; start it again.');
    } finally {
      if (mine === generation) {
        waiting.value = false;
        loginUrl.value = '';
      }
    }
  };

  // Stops waiting; a sign-in completed later in the tab still takes effect
  // on the server.
  const cancel = () => {
    generation++;
    waiting.value = false;
    loginUrl.value = '';
  };

  onBeforeUnmount(cancel);

  return { waiting, loginUrl, openTab, signIn, cancel };
}
