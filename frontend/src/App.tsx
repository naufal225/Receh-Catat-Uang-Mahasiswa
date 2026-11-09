// Route table and guards. Public: landing, login, register. /app/* needs a
// user account; /admin/* needs an admin account.
import { useEffect, type ReactNode } from 'react';
import { AppShell } from './components/AppShell';
import { TransactionModal } from './components/TransactionForm';
import { Logo } from './components/Icon';
import { useAuth } from './lib/auth';
import { navigate, useLocation } from './lib/router';
import AdminAudit from './pages/AdminAudit';
import AdminPromos from './pages/AdminPromos';
import AdminUsers from './pages/AdminUsers';
import Budgets from './pages/Budgets';
import { Login, Register } from './pages/Auth';
import Categories from './pages/Categories';
import Dashboard from './pages/Dashboard';
import Goals from './pages/Goals';
import Landing from './pages/Landing';
import Recurring from './pages/Recurring';
import Reports from './pages/Reports';
import Security from './pages/Security';
import Subscription from './pages/Subscription';
import Transactions from './pages/Transactions';
import Wallets from './pages/Wallets';

const userPages: Record<string, { el: () => ReactNode; title: string }> = {
  '/app': { el: Dashboard, title: 'Ringkasan' },
  '/app/transaksi': { el: Transactions, title: 'Riwayat transaksi' },
  '/app/dompet': { el: Wallets, title: 'Dompet' },
  '/app/kategori': { el: Categories, title: 'Kategori' },
  '/app/anggaran': { el: Budgets, title: 'Anggaran' },
  '/app/target': { el: Goals, title: 'Target tabungan' },
  '/app/rutin': { el: Recurring, title: 'Rutin & rencana' },
  '/app/laporan': { el: Reports, title: 'Laporan bulanan' },
  '/app/keamanan': { el: Security, title: 'Perangkat & keamanan' },
  '/app/langganan': { el: Subscription, title: 'Langganan' },
};
const adminPages: Record<string, { el: () => ReactNode; title: string }> = {
  '/admin': { el: AdminUsers, title: 'Pengguna' },
  '/admin/audit': { el: AdminAudit, title: 'Audit log' },
  '/admin/kode': { el: AdminPromos, title: 'Kode undangan' },
  '/admin/keamanan': { el: Security, title: 'Perangkat saya' },
};

function Redirect({ to }: { to: string }) {
  useEffect(() => navigate(to, { replace: true }), [to]);
  return null;
}

function Splash() {
  return (
    <div style={{ minHeight: '100dvh', display: 'grid', placeItems: 'center' }} aria-busy="true">
      <Logo size={44} />
    </div>
  );
}

export default function App() {
  const { user, ready } = useAuth();
  const { path } = useLocation();
  const clean = path.length > 1 ? path.replace(/\/$/, '') : path;

  const userPage = userPages[clean];
  const adminPage = adminPages[clean];
  const title = userPage?.title ?? adminPage?.title;
  useEffect(() => {
    document.title = title ? `${title} · Receh` : 'Receh · Keuangan mahasiswa';
  }, [title]);

  if (!ready) return <Splash />;

  if (clean === '/') return <Landing />;
  if (clean === '/masuk' || clean === '/daftar') {
    if (user) return <Redirect to={user.role === 'admin' ? '/admin' : '/app'} />;
    return clean === '/masuk' ? <Login /> : <Register />;
  }

  if (userPage || adminPage) {
    if (!user) return <Redirect to="/masuk" />;
    if (userPage && user.role !== 'user') return <Redirect to="/admin" />;
    if (adminPage && user.role !== 'admin') return <Redirect to="/app" />;
    const Page = (userPage ?? adminPage).el;
    return (
      <AppShell>
        <Page />
        {user.role === 'user' && <TransactionModal />}
      </AppShell>
    );
  }

  return (
    <div style={{ minHeight: '100dvh', display: 'grid', placeItems: 'center', textAlign: 'center', padding: 20 }}>
      <div>
        <h1 style={{ fontSize: 64 }}>404</h1>
        <p className="muted" style={{ margin: '8px 0 20px' }}>Halaman ini tidak ditemukan.</p>
        <a className="btn btn-primary" href="/">Kembali ke beranda</a>
      </div>
    </div>
  );
}
