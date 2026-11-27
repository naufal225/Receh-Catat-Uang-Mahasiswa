// Finance module types: mirror backend/internal/modules/finance/model.

export type TxType = 'income' | 'expense';
/** Every row in history: transfers add two legs that are neither income nor expense. */
export type EntryType = TxType | 'transfer_out' | 'transfer_in';
export type WalletType = 'bank' | 'ewallet' | 'cash' | 'other';
export type Frequency = 'daily' | 'weekly' | 'monthly' | 'yearly';
export type Unit = 'day' | 'week' | 'month' | 'year';

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
  importBatchId?: number;
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
  budgets: BudgetStatus[];
  goals: GoalProgress[];
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

/* ---------- Budgets ---------- */
export type BudgetLevel = 'ok' | 'warning' | 'over';

export interface BudgetStatus {
  id: number;
  categoryId: number;
  categoryName: string;
  categoryColor: string;
  amount: number;
  spent: number;
  remaining: number;
  percent: number;
  level: BudgetLevel;
}

export interface BudgetMonth {
  month: string;
  title: string;
  items: BudgetStatus[];
  totalBudget: number;
  totalSpent: number;
  daysLeft: number;
  isCurrent: boolean;
  unbudgeted: number;
}

/* ---------- Savings goals ---------- */
export interface GoalProgress {
  id: number;
  name: string;
  targetAmount: number;
  deadline: string | null;
  walletId: number | null;
  walletName?: string;
  color: string;
  createdAt: string;
  archivedAt: string | null;
  saved: number;
  remaining: number;
  percent: number;
  achieved: boolean;
  daysLeft: number | null;
  perWeek: number;
  perMonth: number;
  overdue: boolean;
  manualSaving: boolean;
}

export interface GoalContribution {
  id: number;
  goalId: number;
  amount: number;
  note: string;
  occurredOn: string;
  createdAt: string;
}

/* ---------- Faster entry ---------- */
export interface QuickDraft {
  type: TxType;
  amount: number;
  name: string;
  walletId: number;
  categoryId: number;
  occurredOn: string;
}

export interface TxSuggestion {
  name: string;
  type: TxType;
  categoryId: number;
  walletId: number;
  amount: number;
  uses: number;
}

export interface StatementMapping {
  date: number;
  description: number;
  amount: number;
  debit: number;
  credit: number;
  headerRow: number;
}

export interface ImportRow {
  line: number;
  date: string;
  name: string;
  amount: number;
  type: TxType;
  categoryId: number;
  duplicate: boolean;
}

export interface ImportPreview {
  headers: string[] | null;
  mapping: StatementMapping;
  skipped: number;
  rows: ImportRow[];
}

export interface ImportBatch {
  id: number;
  walletId: number;
  walletName: string;
  fileName: string;
  rows: number;
  totalIn: number;
  totalOut: number;
  remaining: number;
  createdAt: string;
}
