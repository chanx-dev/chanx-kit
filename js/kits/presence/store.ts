import type { PresenceMember } from './messages';

/**
 * The roster of each scope. The in-memory default is wrong once you run two
 * processes: use the redis-presence-store kit or your own.
 */
export interface PresenceStore {
  /** Record a connection; true when the member became newly present. */
  add(scope: string, connectionId: string, member: PresenceMember): Promise<boolean>;
  /** Forget a connection; the member when that was their last one. */
  discard(scope: string, connectionId: string): Promise<PresenceMember | undefined>;
  /** Everyone present in the scope, once each. */
  members(scope: string): Promise<PresenceMember[]>;
}

/** Process-local: rosters are incomplete with several processes. */
export class InMemoryPresenceStore implements PresenceStore {
  private readonly scopes = new Map<string, Map<string, PresenceMember>>();

  async add(scope: string, connectionId: string, member: PresenceMember) {
    const connections = this.scopes.get(scope) ?? new Map<string, PresenceMember>();
    this.scopes.set(scope, connections);
    const present = [...connections.values()].some((m) => m.id === member.id);
    connections.set(connectionId, member);
    return !present;
  }

  async discard(scope: string, connectionId: string) {
    const connections = this.scopes.get(scope);
    const member = connections?.get(connectionId);
    if (!connections || !member) return undefined;
    connections.delete(connectionId);
    if (connections.size === 0) this.scopes.delete(scope);
    return [...connections.values()].some((m) => m.id === member.id) ? undefined : member;
  }

  async members(scope: string) {
    const seen = new Map<string, PresenceMember>();
    for (const member of this.scopes.get(scope)?.values() ?? []) {
      if (!seen.has(member.id)) seen.set(member.id, member);
    }
    return [...seen.values()];
  }

  /** Drop all state, for tests. */
  clear() {
    this.scopes.clear();
  }
}
