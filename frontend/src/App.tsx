// Route table and guards. Public: landing, login, register. /app/* needs a
// user account; /admin/* needs an admin account. User pages come from the
// core plus every registered module (src/modules/index.ts).
import { Fragment, useEffect, type ComponentType, type ReactNode } from 'react';
import { brand } from './brand';
import { AppShell } from './components/AppShell';
import { Logo } from './components/Icon';
import { useAuth } from './lib/auth';
import { navigate, useLocation } from './lib/router';
import { modules } from './modules';
import type { ModulePage } from './modules/types';
import AdminAudit from './pages/AdminAudit';
import AdminPromos from './pages/AdminPromos';
import AdminUsers from './pages/AdminUsers';
import { Login, Register } from './pages/Auth';
import Home from './pages/Home';
import Invite from './pages/Invite';
import Landing from './pages/Landing';
import NotificationSettings from './pages/NotificationSettings';
import Security from './pages/Security';
import Subscription from './pages/Subscription';
import Team from './pages/Team';
import { takeNext } from './lib/next';

const corePages: Record<string, ModulePage> = {
  '/app': { el: Home, title: 'Beranda' },
  '/app/tim': { el: Team, title: 'Tim' },
  '/app/notifikasi': { el: NotificationSettings, title: 'Notifikasi' },
  '/app/keamanan': { el: Security, title: 'Perangkat & keamanan' },
  '/app/langganan': { el: Subscription, title: 'Langganan' },
};
// Module pages win over the core placeholder home.
const userPages: Record<string, ModulePage> = Object.assign({}, corePages, ...modules.map((m) => m.pages));

const adminPages: Record<string, ModulePage> = {
  '/admin': { el: AdminUsers, title: 'Pengguna' },
  '/admin/audit': { el: AdminAudit, title: 'Audit log' },
  '/admin/kode': { el: AdminPromos, title: 'Kode undangan' },
  '/admin/keamanan': { el: Security, title: 'Perangkat saya' },
};

/** Every module provider, nested in registration order. */
export function ModuleProviders({ children }: { children: ReactNode }) {
  return modules.reduceRight<ReactNode>((inner, m) => {
    const P = m.Provider as ComponentType<{ children: ReactNode }> | undefined;
    return P ? <P>{inner}</P> : inner;
  }, children);
}

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
    document.title = title ? `${title} · ${brand.name}` : `${brand.name} · ${brand.tagline}`;
  }, [title]);

  // A page waiting for sign-in (an invitation link): open it once signed in,
  // whichever way that happened (password, register or Google).
  useEffect(() => {
    if (!ready || !user) return;
    const next = takeNext();
    if (next) navigate(next, { replace: true });
  }, [ready, user]);

  if (!ready) return <Splash />;

  if (clean === '/undangan') return <Invite />;

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
        {user.role === 'user' && modules.map((m) => m.Overlay && <Fragment key={m.id}><m.Overlay /></Fragment>)}
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
