// Layout for signed-in pages: sidebar on desktop, top bar + bottom nav with a
// central "+" on mobile. Also hosts the theme toggle and the frozen banner.
import { useEffect, type ReactNode } from 'react';
import { useAuth } from '../lib/auth';
import { useFinance } from '../lib/finance';
import { useStoredState } from '../lib/hooks';
import { navigate, useLocation } from '../lib/router';
import { Icon, Logo, type IconName } from './Icon';
import { initial, Link } from './ui';

type NavItem = { to: string; label: string; icon: IconName; short?: string };

const userNav: NavItem[] = [
  { to: '/app', label: 'Ringkasan', icon: 'home', short: 'Ringkasan' },
  { to: '/app/transaksi', label: 'Riwayat transaksi', icon: 'list', short: 'Riwayat' },
  { to: '/app/dompet', label: 'Dompet', icon: 'wallet', short: 'Dompet' },
  { to: '/app/rutin', label: 'Rutin & rencana', icon: 'repeat', short: 'Rutin' },
  { to: '/app/kategori', label: 'Kategori', icon: 'tag' },
  { to: '/app/laporan', label: 'Laporan bulanan', icon: 'file' },
  { to: '/app/keamanan', label: 'Perangkat & keamanan', icon: 'device' },
  { to: '/app/langganan', label: 'Langganan', icon: 'sparkle' },
];

const adminNav: NavItem[] = [
  { to: '/admin', label: 'Pengguna', icon: 'users', short: 'Pengguna' },
  { to: '/admin/audit', label: 'Audit log', icon: 'shield', short: 'Audit' },
  { to: '/admin/kode', label: 'Kode undangan', icon: 'sparkle', short: 'Kode' },
  { to: '/admin/keamanan', label: 'Perangkat saya', icon: 'device', short: 'Perangkat' },
];

function ThemeToggle() {
  const [theme, setTheme] = useStoredState<'light' | 'dark' | ''>('receh-theme', '');
  useEffect(() => {
    if (theme) document.documentElement.dataset.theme = theme;
    else delete document.documentElement.dataset.theme;
  }, [theme]);
  const isDark = theme === 'dark' || (!theme && window.matchMedia('(prefers-color-scheme: dark)').matches);
  return (
    <button type="button" className="icon-btn" onClick={() => setTheme(isDark ? 'light' : 'dark')} aria-label={isDark ? 'Pakai tema terang' : 'Pakai tema gelap'} title="Ganti tema">
      <Icon name={isDark ? 'sun' : 'moon'} />
    </button>
  );
}

export function AppShell({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const { openTx, readOnly, membership } = useFinance();
  const { path } = useLocation();
  const isAdmin = user?.role === 'admin';
  // "Langganan" only exists where something is sold (billing on).
  const nav = isAdmin ? adminNav : userNav.filter((n) => n.to !== '/app/langganan' || membership?.billingEnabled);
  const isPremium = !isAdmin && membership?.plan === 'premium' && membership.billingEnabled;
  const current = (to: string) => (to === '/app' || to === '/admin' ? path === to : path.startsWith(to));

  // "N" opens the quick-add form from anywhere, unless the user is typing.
  useEffect(() => {
    if (isAdmin || readOnly) return;
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (e.key.toLowerCase() !== 'n' || e.metaKey || e.ctrlKey || e.altKey) return;
      if (t.closest('input, textarea, select, [contenteditable], dialog')) return;
      e.preventDefault();
      openTx();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isAdmin, readOnly, openTx]);

  async function signOut() {
    await logout();
    navigate('/masuk', { replace: true });
  }

  const mobileItems = nav.filter((n) => n.short);

  return (
    <div className="shell">
      <aside className="sidebar" aria-label="Navigasi utama">
        <Link to={isAdmin ? '/admin' : '/app'} className="brand">
          <Logo /> Receh
        </Link>
        {!isAdmin && (
          <button type="button" className="btn btn-accent" style={{ marginBottom: 14 }} onClick={() => openTx()} disabled={readOnly} aria-keyshortcuts="N" title="Pintasan keyboard: N">
            <Icon name="plus" size={18} /> Catat transaksi
          </button>
        )}
        {isAdmin && <div className="nav-section">Panel admin</div>}
        {nav.map((n) => (
          <Link key={n.to} to={n.to} className="nav-link" aria-current={current(n.to) ? 'page' : undefined}>
            <Icon name={n.icon} /> {n.label}
          </Link>
        ))}
        <div className="sidebar-foot">
          <div className="user-chip">
            <div className="avatar">{initial(user?.name ?? '')}</div>
            <div>
              <strong>{user?.name}{isPremium && <span className="badge badge-accent premium-tag"><Icon name="sparkle" size={11} /> Premium</span>}</strong>
              <span>{user?.email}</span>
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <ThemeToggle />
            <button type="button" className="btn btn-quiet btn-sm" onClick={signOut}>
              <Icon name="logout" size={18} /> Keluar
            </button>
          </div>
        </div>
      </aside>

      <header className="mobile-top">
        <Link to={isAdmin ? '/admin' : '/app'} className="brand">
          <Logo size={26} /> Receh
        </Link>
        <div style={{ display: 'flex', gap: 2 }}>
          {!isAdmin && (
            <>
              <Link to="/app/kategori" className="icon-btn" aria-label="Kategori"><Icon name="tag" /></Link>
              <Link to="/app/laporan" className="icon-btn" aria-label="Laporan"><Icon name="file" /></Link>
              <Link to="/app/keamanan" className="icon-btn" aria-label="Perangkat & keamanan"><Icon name="device" /></Link>
              {membership?.billingEnabled && <Link to="/app/langganan" className="icon-btn" aria-label="Langganan"><Icon name="sparkle" /></Link>}
            </>
          )}
          <ThemeToggle />
          <button type="button" className="icon-btn" onClick={signOut} aria-label="Keluar"><Icon name="logout" /></button>
        </div>
      </header>

      <main className="main" id="main">
        {readOnly && (
          <div className="banner" role="status">
            <Icon name="snow" />
            <div>
              <strong>Akun Anda sedang dibekukan.</strong> Anda masih bisa melihat data dan mengunduh laporan, tetapi tidak bisa mencatat atau mengubah apa pun.
              {user?.statusReason && <> Alasan: {user.statusReason}</>}
            </div>
          </div>
        )}
        {children}
      </main>

      <nav className="bottom-nav" aria-label="Navigasi bawah" style={isAdmin ? { gridTemplateColumns: `repeat(${mobileItems.length}, 1fr)` } : undefined}>
        {mobileItems.slice(0, 2).map((n) => (
          <Link key={n.to} to={n.to} aria-current={current(n.to) ? 'page' : undefined}>
            <Icon name={n.icon} /> {n.short}
          </Link>
        ))}
        {!isAdmin && (
          <button type="button" onClick={() => openTx()} disabled={readOnly} aria-label="Catat transaksi">
            <span className="fab"><Icon name="plus" size={26} /></span>
            Catat
          </button>
        )}
        {mobileItems.slice(2).map((n) => (
          <Link key={n.to} to={n.to} aria-current={current(n.to) ? 'page' : undefined}>
            <Icon name={n.icon} /> {n.short}
          </Link>
        ))}
      </nav>
    </div>
  );
}
