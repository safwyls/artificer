import { api } from "./api";
import type { AppUser } from "./types";

/** SYNC_PERM is the grant that separates an account that can hold a world
 * from one that can only look at it. Admins have it implicitly. */
export const SYNC_PERM = "savesync";

export const hasSync = (u: AppUser) => (u.permissions ?? []).includes(SYNC_PERM);

export interface UserChanges {
  role?: string;
  /** true grants the custody permission, false revokes it, undefined
   * leaves it as it is. */
  sync?: boolean;
  disabled?: boolean;
}

/**
 * saveUser sends the user's **whole record** with one field changed,
 * because that is what the API replaces. The update endpoint writes role,
 * permissions and disabled together, so a button that sent only its own
 * field would quietly clear the other two — which is how "Disable" used to
 * be written, and how a user could lose world custody by being disabled
 * and re-enabled.
 */
export function saveUser(u: AppUser, changes: UserChanges) {
  const perms = new Set(u.permissions ?? []);
  if (changes.sync === true) perms.add(SYNC_PERM);
  if (changes.sync === false) perms.delete(SYNC_PERM);
  return api.updateUser(u.id, {
    role: changes.role !== undefined ? changes.role : u.role,
    permissions: [...perms],
    disabled: changes.disabled !== undefined ? changes.disabled : Boolean(u.disabled),
  });
}
