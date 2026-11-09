// Mirrors the JSON the Go API returns (backend/internal/domain/entities.go).

export type Role = 'user' | 'admin';
export type UserStatus = 'active' | 'suspended' | 'frozen';
export type TxType = 'income' | 'expense';
/** Every row in history: transfers add two legs that are neither income nor expense. */
export type EntryType = TxType | 'transfer_out' | 'transfer_in';
export type WalletType = 'bank' | 'ewallet' | 'cash' | 'other';
export type Frequency = 'daily' | 'weekly' | 'monthly' | 'yearly';
export type Unit = 'day' | 'week' | 'month' | 'year';

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

export interface Wallet {
  id: number;
  name: string;
  type: WalletType;
  color: string;
  balance: number;
  archived: boolean;
}

export interface Category {
  id: number;
  name: string;
  type: TxType;
  color: string;
}

export interface Transaction {
  id: number;
  walletId: number;
  walletName: string;
  walletColor: string;
  categoryId: number;
  categoryName: string;
  categoryColor: string;
  type: EntryType;
  name: string;
  amount: number;
  occurredOn: string;
  recurringId: number | null;
  /** Set on both legs of a transfer and on its fee expense. */
  transferId: number | null;
  counterpartName?: string;
  counterpartColor?: string;
  transferFee?: number;
  createdAt: string;
}

export interface Transfer {
  id: number;
  fromWalletId: number;
  fromWalletName: string;
  toWalletId: number;
  toWalletName: string;
  amount: number;
  fee: number;
  note: string;
  occurredOn: string;
  createdAt: string;
}

export interface Recurring {
  id: number;
  walletId: number;
  walletName: string;
  walletColor: string;
  categoryId: number;
  categoryName: string;
  categoryColor: string;
  type: TxType;
  name: string;
  amount: number;
  frequency: Frequency;
  anchorDay: number;
  nextDue: string;
  active: boolean;
  lastError: string;
}

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export interface Bucket {
  key: string;
  label: string;
  from: string;
  to: string;
  income: number;
  expense: number;
}

export interface CategorySlice {
  categoryId: number;
  name: string;
  color: string;
  total: number;
}

export interface Totals {
  income: number;
  expense: number;
}

export interface Dashboard {
  unit: Unit;
  from: string;
  to: string;
  title: string;
  canGoForward: boolean;
  buckets: Bucket[];
  totals: Totals;
  expenseSlices: CategorySlice[];
  incomeSlices: CategorySlice[];
  totalBalance: number;
  wallets: Wallet[];
  latest: Transaction[];
  upcomingRoutines: Recurring[];
}

export interface SliceSet {
  totals: Totals;
  expenseSlices: CategorySlice[];
  incomeSlices: CategorySlice[];
}

export interface WalletSummary {
  name: string;
  income: number;
  expense: number;
  transferIn: number;
  transferOut: number;
  balance: number;
}

export interface MonthlyReport {
  month: string;
  title: string;
  owner: string;
  totals: Totals;
  wallets: WalletSummary[];
  expenseSlices: CategorySlice[];
  incomeSlices: CategorySlice[];
  transactions: Transaction[];
  generatedAt: string;
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

export interface Limits {
  /** -1 = tanpa batas */
  maxWallets: number;
  maxBudgets: number;
  maxGoals: number;
  csvImport: boolean;
}

export interface Membership {
  plan: Plan;
  /** False on the personal edition: everyone is premium and nothing is sold. */
  billingEnabled: boolean;
  source?: EntitlementSource;
  endsAt: string | null;
  note?: string;
  limits: Limits;
  freeLimits: Limits;
  usage: { wallets: number; budgets: number; goals: number };
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
