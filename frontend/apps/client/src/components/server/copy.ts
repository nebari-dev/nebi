// Owner: U5 (remote). Prototype copy shared by the server screens.
import type { SyncMarker } from '@/types/contracts';

export const DISCONNECT_COPY =
  'Severs the link to this server. Projects you’ve already pulled stay on your machine and keep working. They just stop syncing. You can reconnect at any time.';

/** Placeholder shown for the stored token in "Manage connection". */
export const MASKED_TOKEN = '••••••••••••••••';

export const nameTakenMessage = (name: string) =>
  `A project called ${name} is already on this machine. Pick another name.`;

export const SYNC_MARKER_TIPS: Record<SyncMarker, string> = {
  newest_here: 'Latest on this machine. Not on the server yet.',
  newest_on_server: 'Latest on the server. Not on this machine yet.',
  in_sync: 'This machine and the server both have this version.',
};
