// The finance example module: wallets, transactions, transfers, budgets,
// goals, recurring entries, reports and CSV import. Remove it by deleting this
// folder and its line in src/modules/index.ts (and the backend module).
import { TransactionModal } from './components/TransactionForm';
import { FinanceProvider, useFinance } from './context';
import Budgets from './pages/Budgets';
import Categories from './pages/Categories';
import Dashboard from './pages/Dashboard';
import Goals from './pages/Goals';
import Import from './pages/Import';
import Recurring from './pages/Recurring';
import Reports from './pages/Reports';
import Transactions from './pages/Transactions';
import Wallets from './pages/Wallets';
import type { AppModule } from '../types';

export const finance: AppModule = {
  id: 'finance',
  pages: {
    '/app': { el: Dashboard, title: 'Ringkasan' },
    '/app/transaksi': { el: Transactions, title: 'Riwayat transaksi' },
    '/app/dompet': { el: Wallets, title: 'Dompet' },
    '/app/kategori': { el: Categories, title: 'Kategori' },
    '/app/anggaran': { el: Budgets, title: 'Anggaran' },
    '/app/target': { el: Goals, title: 'Target tabungan' },
    '/app/impor': { el: Import, title: 'Impor mutasi' },
    '/app/rutin': { el: Recurring, title: 'Rutin & rencana' },
    '/app/laporan': { el: Reports, title: 'Laporan bulanan' },
  },
  nav: [
    { to: '/app', label: 'Ringkasan', icon: 'home', short: 'Ringkasan' },
    { to: '/app/transaksi', label: 'Riwayat transaksi', icon: 'list', short: 'Riwayat' },
    { to: '/app/dompet', label: 'Dompet', icon: 'wallet', short: 'Dompet' },
    { to: '/app/anggaran', label: 'Anggaran', icon: 'pie', short: 'Anggaran' },
    { to: '/app/target', label: 'Target tabungan', icon: 'target' },
    { to: '/app/rutin', label: 'Rutin & rencana', icon: 'repeat' },
    { to: '/app/impor', label: 'Impor mutasi', icon: 'upload' },
    { to: '/app/kategori', label: 'Kategori', icon: 'tag' },
    { to: '/app/laporan', label: 'Laporan bulanan', icon: 'file' },
  ],
  Provider: FinanceProvider,
  Overlay: TransactionModal,
  primary: {
    label: 'Catat transaksi',
    short: 'Catat',
    icon: 'plus',
    shortcut: 'n',
    use: () => {
      const { openTx, readOnly } = useFinance();
      return { run: () => openTx(), disabled: readOnly };
    },
  },
};
