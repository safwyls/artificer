export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

/** Registered by the auth provider. A 401 from any endpoint means the
 * session expired (or was revoked) mid-use; clearing auth state here lets
 * RequireAuth bounce to /login once instead of every query surfacing its
 * own scattered error. */
let onUnauthorized: (() => void) | null = null;

export function setUnauthorizedHandler(handler: (() => void) | null) {
  onUnauthorized = handler;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });

  if (res.status === 204) {
    return undefined as T;
  }

  const isJSON = res.headers.get("content-type")?.includes("application/json");
  const body = isJSON ? await res.json() : undefined;

  if (!res.ok) {
    // /login's own 401 is just a wrong password, not an expired session.
    if (res.status === 401 && path !== "/login") {
      onUnauthorized?.();
    }
    const message = body && typeof body === "object" && "error" in body ? String(body.error) : res.statusText;
    throw new ApiError(res.status, message);
  }
  return body as T;
}

/** The server's explanation for a failed request — for toasts that would
 * otherwise say what failed but not why. */
export function errorDetail(err: unknown): string | undefined {
  return err instanceof ApiError && err.message ? err.message : undefined;
}

export const PERMISSIONS = ["power", "broadcast", "save", "moderate", "shutdown", "settings"] as const;
export type Permission = (typeof PERMISSIONS)[number];

/** Human labels for the permission checkboxes, and what each actually allows. */
export const PERMISSION_LABELS: Record<Permission, { label: string; help: string }> = {
  power: { label: "Power", help: "Start, stop and restart the server container" },
  broadcast: { label: "Broadcast", help: "Send in-game messages" },
  save: { label: "Save world", help: "Trigger a world save" },
  moderate: { label: "Moderate", help: "Kick, ban and unban players" },
  shutdown: { label: "In-game shutdown", help: "Shut the server down with a countdown" },
  settings: { label: "Edit settings", help: "Read and edit PalWorldSettings.ini" },
};

export interface Me {
  username: string;
  role: string;
  isAdmin: boolean;
  permissions: Permission[];
}

export interface AppUser {
  id: number;
  username: string;
  role: string;
  permissions: Permission[];
  disabled: boolean;
}

export interface UserWriteInput {
  username?: string;
  password?: string;
  role: string;
  permissions: Permission[];
  disabled?: boolean;
}

export interface ContainerState {
  name: string;
  status: string;
  running: boolean;
  startedAt: string;
  exitCode: number;
}

export interface Server {
  id: number;
  name: string;
  host: string;
  rconPort: number;
  hasRconPassword: boolean;
  restPort: number;
  hasRestPassword: boolean;
  useRest: boolean;
  enabled: boolean;
  savePath: string;
  configPath: string;
  containerName: string;
}

export interface ServerWriteInput {
  name: string;
  host: string;
  rconPort: number;
  rconPassword?: string;
  restPort: number;
  restPassword?: string;
  useRest: boolean;
  enabled: boolean;
  savePath: string;
  configPath: string;
  containerName: string;
}

export interface ServerInfo {
  servername: string;
  version: string;
  playerCount: number;
  transport: "rest" | "rcon";
}

export interface Player {
  name: string;
  playerId: string;
  userId: string;
  level: number;
  ping: number;
  location_x: number;
  location_y: number;
}

export interface Metrics {
  serverfps: number;
  serverframetime: number;
  currentplayernum: number;
  maxplayernum: number;
  uptime: number;
  days: number;
}

export type Settings = Record<string, unknown>;

/** One editable PalWorldSettings.ini option. `type` decides which control the
 * editor renders; `value` is the decoded display value (strings unquoted). */
export interface ConfigSetting {
  key: string;
  value: string;
  type: "bool" | "int" | "float" | "string" | "enum";
}

export interface ConfigResult {
  settings: ConfigSetting[];
  /** Resolved path of the PalWorldSettings.ini that was read. */
  path: string;
  /** False when the config file is on a read-only mount — edits will fail. */
  writable: boolean;
}

/** One collected sample. Nulls are real gaps — the server was unreachable
 * or reported nothing — and must break the line rather than plot as zero. */
export interface MetricPoint {
  ts: string;
  playerCount: number | null;
  maxPlayers: number | null;
  serverFps: number | null;
  frameTime: number | null;
}

export interface MetricsHistory {
  points: MetricPoint[];
  /** Collection cadence, so the chart can tell a gap from sparse sampling. */
  intervalSeconds: number;
}

export interface Pal {
  instanceId: string;
  characterId: string;
  nickname: string;
  level: number;
  gender: "male" | "female" | "";
  isBoss: boolean;
  isLucky: boolean;
  rank: number;
  talentHp: number;
  talentShot: number;
  talentDefense: number;
  passives: string[];
  exp: number;
  skills: string[];
  hp: number;
  sanity: number;
  stomach: number;
  friendship: number;
  /** Ailment name, or "" when healthy. A sick pal stops working at a base. */
  sick: string;
  /** Soul upgrades applied, keyed by stat name. */
  souls: Record<string, number>;
  slotIndex: number;
  /** The base camp a working pal belongs to (matches a guild base's id);
   * empty for pals not working at a base. */
  baseId: string;
}

export interface PlayerPals {
  uid: string;
  nickname: string;
  level: number;
  party: Pal[];
  palbox: Pal[];
  base: Pal[];
  /** Dimensional Pal Storage, plus this player's share of the global storage. */
  storage: Pal[];
  /** Unix seconds; 0 when the save recorded none. */
  lastOnline: number;
  /** Where they logged off, in the same world space the map plots. */
  lastX: number | null;
  lastY: number | null;
  platform: string;
  technologyPoints: number;
  /** Paldex progress: registered species ids (survive selling the pal) and
   * per-species sphere-capture counts, from the player's save record. */
  paldeck: string[];
  captures: Record<string, number>;
}

export interface PlayerEvent {
  id: number;
  ts: string;
  userId: string;
  name: string;
  event: "join" | "leave";
}

export interface ActivityResult {
  events: PlayerEvent[];
  hours: number;
  /** Sampling cadence — session edges are only this precise. */
  intervalSeconds: number;
}

export interface AuditEntry {
  id: number;
  ts: string;
  username: string;
  action: string;
  detail: string;
}

export interface RestartSchedule {
  id: number;
  enabled: boolean;
  /** Weekdays, 0 (Sunday) through 6 (Saturday). */
  days: number[];
  /** "HH:MM", 24-hour, in Palcon's local timezone. */
  timeOfDay: string;
  /** Warning broadcast lead times in minutes, descending. */
  warningMinutes: number[];
  lastRunAt: string | null;
  nextRunAt: string | null;
}

export interface ScheduleWriteInput {
  enabled: boolean;
  days: number[];
  timeOfDay: string;
  warningMinutes: number[];
}

/** The webhook URL itself is write-only; the API only ever reports that
 * one is configured. */
export interface DiscordConfig {
  configured: boolean;
  enabled: boolean;
  onStatus: boolean;
  onPlayers: boolean;
  onRestarts: boolean;
}

export interface DiscordWriteInput {
  /** Empty string keeps the stored webhook (like password updates). */
  webhookUrl: string;
  enabled: boolean;
  onStatus: boolean;
  onPlayers: boolean;
  onRestarts: boolean;
}

export interface AutomationResult {
  schedules: RestartSchedule[];
  /** Palcon's local timezone name, which schedule times are read in. */
  timezone: string;
  /** True when a scheduled restart can bounce the container itself. */
  dockerRestart: boolean;
  /** Absent for non-admins. */
  discord?: DiscordConfig;
  /** Absent for non-admins. `available` = docker control + container name. */
  watchdog?: { enabled: boolean; available: boolean };
  /** Absent for non-admins. Token is the /status/<token> URL segment. */
  publicStatus?: { enabled: boolean; token: string };
}

export interface BackupSnapshot {
  name: string;
  ts: string;
  bytes: number;
}

export interface BackupsResult {
  /** False when the server has no save path to snapshot. */
  available: boolean;
  running: boolean;
  /** 0 = no schedule; manual backups still work. */
  intervalHours: number;
  keep: number;
  snapshots: BackupSnapshot[];
  totalBytes: number;
}

/** The unauthenticated status snapshot behind a public token. */
export interface PublicStatus {
  name: string;
  online: boolean;
  players?: number;
  maxPlayers?: number;
  nextRestartAt?: string;
}

export interface GuildMember {
  uid: string;
  name: string;
}

export interface Guild {
  id: string;
  name: string;
  baseCampLevel: number;
  members: GuildMember[];
  memberCount: number;
  bases: { id: string; x: number; y: number }[];
}

export interface GuildsResult {
  guilds: Guild[];
  players: PlayerPals[];
  parsedAt: string;
  saveModTime: string;
}

export interface PalsResult {
  players: PlayerPals[];
  guilds: Guild[];
  parsedAt: string;
  saveModTime: string;
}

export const api = {
  login: (username: string, password: string) =>
    request<{ username: string }>("/login", { method: "POST", body: JSON.stringify({ username, password }) }),
  logout: () => request<void>("/logout", { method: "POST" }),
  me: () => request<Me>("/me"),
  changeOwnPassword: (currentPassword: string, newPassword: string) =>
    request<void>("/me/password", { method: "POST", body: JSON.stringify({ currentPassword, newPassword }) }),

  listUsers: () => request<AppUser[]>("/users"),
  createUser: (input: UserWriteInput) => request<AppUser>("/users", { method: "POST", body: JSON.stringify(input) }),
  updateUser: (id: number, input: UserWriteInput) =>
    request<AppUser>(`/users/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteUser: (id: number) => request<void>(`/users/${id}`, { method: "DELETE" }),

  containerStatus: (id: number) => request<ContainerState>(`/servers/${id}/container`),
  containerAction: (id: number, action: "start" | "stop" | "restart") =>
    request<ContainerState>(`/servers/${id}/container/${action}`, { method: "POST" }),
  // Needs the power permission, like the actions beside it.
  containerLogs: (id: number, tail: number) =>
    request<{ lines: string[] }>(`/servers/${id}/container/logs?tail=${tail}`),
  setWatchdog: (id: number, enabled: boolean) =>
    request<{ enabled: boolean }>(`/servers/${id}/watchdog`, { method: "PUT", body: JSON.stringify({ enabled }) }),
  setPublicStatus: (id: number, enabled: boolean) =>
    request<{ enabled: boolean; token: string }>(`/servers/${id}/public`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  publicStatus: (token: string) => request<PublicStatus>(`/public/status/${token}`),

  // Save backups — admin-only end to end (a snapshot is the whole world).
  listBackups: (id: number) => request<BackupsResult>(`/servers/${id}/backups`),
  setBackupSettings: (id: number, intervalHours: number, keep: number) =>
    request<{ intervalHours: number; keep: number }>(`/servers/${id}/backups/settings`, {
      method: "PUT",
      body: JSON.stringify({ intervalHours, keep }),
    }),
  runBackup: (id: number) => request<void>(`/servers/${id}/backups/run`, { method: "POST" }),
  deleteBackup: (id: number, name: string) => request<void>(`/servers/${id}/backups/${name}`, { method: "DELETE" }),
  backupDownloadURL: (id: number, name: string) => `/api/servers/${id}/backups/${name}/download`,

  listServers: () => request<Server[]>("/servers"),
  getServer: (id: number) => request<Server>(`/servers/${id}`),
  createServer: (input: ServerWriteInput) => request<Server>("/servers", { method: "POST", body: JSON.stringify(input) }),
  updateServer: (id: number, input: ServerWriteInput) =>
    request<Server>(`/servers/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteServer: (id: number) => request<void>(`/servers/${id}`, { method: "DELETE" }),

  serverInfo: (id: number) => request<ServerInfo>(`/servers/${id}/info`),
  serverPlayers: (id: number) => request<Player[]>(`/servers/${id}/players`),
  broadcast: (id: number, message: string) =>
    request<void>(`/servers/${id}/broadcast`, { method: "POST", body: JSON.stringify({ message }) }),
  kick: (id: number, playerUid: string, message: string) =>
    request<void>(`/servers/${id}/kick`, { method: "POST", body: JSON.stringify({ playerUid, message }) }),
  ban: (id: number, playerUid: string, message: string) =>
    request<void>(`/servers/${id}/ban`, { method: "POST", body: JSON.stringify({ playerUid, message }) }),
  unban: (id: number, playerUid: string) =>
    request<void>(`/servers/${id}/unban`, { method: "POST", body: JSON.stringify({ playerUid }) }),
  save: (id: number) => request<void>(`/servers/${id}/save`, { method: "POST" }),
  shutdown: (id: number, waitSeconds: number, message: string) =>
    request<void>(`/servers/${id}/shutdown`, { method: "POST", body: JSON.stringify({ waitSeconds, message }) }),

  // REST-only — throws a 400 ApiError for servers configured RCON-only.
  serverSettings: (id: number) => request<Settings>(`/servers/${id}/settings`),

  // PalWorldSettings.ini editor (needs the "settings" permission). Throws a
  // 400 ApiError when the server has no config path configured.
  serverConfig: (id: number) => request<ConfigResult>(`/servers/${id}/config`),
  updateServerConfig: (id: number, changes: Record<string, string>) =>
    request<ConfigResult>(`/servers/${id}/config`, { method: "PUT", body: JSON.stringify({ changes }) }),
  serverMetrics: (id: number) => request<Metrics>(`/servers/${id}/metrics`),
  serverMetricsHistory: (id: number, minutes: number) =>
    request<MetricsHistory>(`/servers/${id}/metrics/history?minutes=${minutes}`),

  // Save-file-backed (phase 5) — throws a 400 ApiError when the server has
  // no save path configured.
  serverPals: (id: number) => request<PalsResult>(`/servers/${id}/pals`),
  serverGuilds: (id: number) => request<GuildsResult>(`/servers/${id}/guilds`),

  // Activity: join/leave history for anyone signed in; the audit trail of
  // management actions is admin-only.
  serverActivity: (id: number, hours: number) => request<ActivityResult>(`/servers/${id}/activity?hours=${hours}`),
  serverAudit: (id: number, limit = 200) => request<{ entries: AuditEntry[] }>(`/servers/${id}/audit?limit=${limit}`),

  // Automation: restart schedules (readable by anyone signed in) and
  // Discord notifications (admin-only, and part of the same payload).
  serverAutomation: (id: number) => request<AutomationResult>(`/servers/${id}/automation`),
  createSchedule: (id: number, input: ScheduleWriteInput) =>
    request<RestartSchedule>(`/servers/${id}/schedules`, { method: "POST", body: JSON.stringify(input) }),
  updateSchedule: (id: number, scheduleId: number, input: ScheduleWriteInput) =>
    request<RestartSchedule>(`/servers/${id}/schedules/${scheduleId}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteSchedule: (id: number, scheduleId: number) =>
    request<void>(`/servers/${id}/schedules/${scheduleId}`, { method: "DELETE" }),
  setDiscord: (id: number, input: DiscordWriteInput) =>
    request<DiscordConfig>(`/servers/${id}/discord`, { method: "PUT", body: JSON.stringify(input) }),
  deleteDiscord: (id: number) => request<void>(`/servers/${id}/discord`, { method: "DELETE" }),
  testDiscord: (id: number) => request<void>(`/servers/${id}/discord/test`, { method: "POST" }),
};
