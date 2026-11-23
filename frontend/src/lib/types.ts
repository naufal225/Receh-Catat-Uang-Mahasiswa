// Mirrors the JSON the Go core API returns (backend/internal/domain).
// Module types live with their module (src/modules/<name>/types.ts).

export type Role = 'user' | 'admin';
export type UserStatus = 'active' | 'suspended' | 'frozen';

/** A label in every UI language, as modules declare them on the server. */
export type Localized = Record<string, string>;

export interface User {
  id: number;
  name: string;
  email: string;
  role: Role;
  status: UserStatus;
  statusReason: string;
  createdAt: string;
  /** False for accounts created with Google that never set a password. */
  hasPassword: boolean;
  googleLinked: boolean;
  /** Only in the admin user list. */
  plan?: Plan;
}

export interface Session {
  id: number;
  ip: string;
  device: string;
  location: string;
  createdAt: string;
  lastSeenAt: string;
  expiresAt: string;
  current: boolean;
}

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export interface AuditLog {
  id: number;
  userId: number | null;
  email: string;
  event: string;
  ip: string;
  device: string;
  location: string;
  detail: string;
  createdAt: string;
}

/* ---------- Membership (freemium) ---------- */
export type Plan = 'free' | 'premium';
export type EntitlementSource = 'paid' | 'comp' | 'code';

/** One limit a module enforces (e.g. "finance.wallets"). */
export interface LimitDef {
  key: string;
  label: Localized;
  /** On/off feature (0 or 1) rather than a count. */
  feature: boolean;
  free: number;
  premium: number;
}

/** Value per LimitDef key. -1 = unlimited; for features 0 = off, 1 = on. */
export type Limits = Record<string, number>;

export interface Membership {
  /** The workspace described, and whether the viewer owns (pays for) it. */
  orgId: number;
  isOwner: boolean;
  plan: Plan;
  /** False on the personal edition: everyone is premium and nothing is sold. */
  billingEnabled: boolean;
  source?: EntitlementSource;
  endsAt: string | null;
  note?: string;
  limits: Limits;
  freeLimits: Limits;
  usage: Record<string, number>;
  defs: LimitDef[];
}

export interface Entitlement {
  id: number;
  userId: number;
  source: EntitlementSource;
  plan: Plan;
  startsAt: string;
  endsAt: string | null;
  note: string;
  promoCode?: string;
  revokedAt: string | null;
  createdAt: string;
}

export interface PromoCode {
  id: number;
  code: string;
  plan: Plan;
  durationDays: number | null;
  maxUses: number;
  used: number;
  expiresAt: string | null;
  note: string;
  active: boolean;
  createdAt: string;
}

/* ---------- Notifications ---------- */
/** Kinds are declared by modules (see NotificationKindDef). */
export type NotificationKind = string;

export interface NotificationKindDef {
  key: NotificationKind;
  label: Localized;
  icon: string;
  tone: 'ok' | 'warn' | 'danger' | 'neutral';
}

export interface AppNotification {
  id: number;
  kind: NotificationKind;
  title: string;
  body: string;
  link: string;
  readAt: string | null;
  createdAt: string;
}

export interface Inbox {
  items: AppNotification[];
  unread: number;
  kinds: NotificationKindDef[];
}

export interface NotificationSetting {
  key: string;
  label: Localized;
  /** How to edit it: amount (money), hour (0-23) or number. */
  kind: 'amount' | 'hour' | 'number';
  default: number;
  min: number;
  max: number;
}

export interface NotificationPrefs {
  kinds: (NotificationKindDef & { enabled: boolean })[];
  settings: (NotificationSetting & { value: number })[];
}

/* ---------- Workspaces (organizations) ---------- */
export type OrgRole = 'owner' | 'admin' | 'member';

export interface OrgMembership {
  id: number;
  name: string;
  /** Every account has one personal workspace; it cannot have other members. */
  personal: boolean;
  ownerId: number;
  createdAt: string;
  role: OrgRole;
}

export interface OrgPerson {
  userId: number;
  name: string;
  email: string;
  role: OrgRole;
  joinedAt: string;
}

export interface OrgInvite {
  id: number;
  orgId: number;
  orgName?: string;
  email: string;
  role: OrgRole;
  invitedBy: string;
  expiresAt: string;
  createdAt: string;
}
