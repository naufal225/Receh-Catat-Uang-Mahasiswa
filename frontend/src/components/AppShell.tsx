// Layout for signed-in pages: sidebar on desktop, top bar + bottom nav with a
// central "+" on mobile. Also hosts the theme toggle and the frozen banner.
import { useEffect, type ReactNode } from 'react';
import { useAuth } from '../lib/auth';
import { useFinance } from '../lib/finance';
import { useStoredState } from '../lib/hooks';
import { navigate, useLocation } from '../lib/router';
import { Icon, Logo, type IconName } from './Icon';
import { initial, Link } from './ui';

type NavItem = { to: string; label: string; icon: IconName; short?: string; group: string };

const userNav: NavItem[] = [
  { to: '/app', label: 'Ringkasan', icon: 'home', short: 'Ringkasan', group: 'Utama' },
  { to: '/app/transaksi', label: 'Riwayat transaksi', icon: 'list', short: 'Riwayat', group: 'Utama' },
  { to: '/app/dompet', label: 'Dompet', icon: 'wallet', short: 'Dompet', group: 'Kelola' },
  { to: '/app/rutin', label: 'Rutin & rencana', icon: 'repeat', short: 'Rutin', group: 'Kelola' },
  { to: '/app/kategori', label: 'Kategori', icon: 'tag', group: 'Kelola' },
  { to: '/app/laporan', label: 'Laporan bulanan', icon: 'file', group: 'Laporan' },
  { to: '/app/keamanan', label: 'Perangkat & keamanan', icon: 'device', group: 'Akun' },
];

const adminNav: NavItem[] = [
  { to: '/admin', label: 'Pengguna', icon: 'users', short: 'Pengguna', group: 'Panel admin' },
  { to: '/admin/audit', label: 'Audit log', icon: 'shield', short: 'Audit', group: 'Panel admin' },
  { to: '/admin/keamanan', label: 'Perangkat saya', icon: 'device', short: 'Perangkat', group: 'Akun' },
];

// Consecutive items with the same group share one labelled section.
function groupNav(items: NavItem[]): { label: string; items: NavItem[] }[] {
  const out: { label: string; items: NavItem[] }[] = [];
  for (const n of items) {
    const last = out[out.length - 1];
    if (last && last.label === n.group) last.items.push(n);
    else out.push({ label: n.group, items: [n] });
  }
  return out;
}

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
  const { openTx, readOnly } = useFinance();
  const { path } = useLocation();
  const isAdmin = user?.role === 'admin';
  const nav = isAdmin ? adminNav : userNav;
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
  const sections = groupNav(nav);

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
        <nav className="side-nav" aria-label="Menu">
          {sections.map((sec) => (
            <section key={sec.label} className="nav-group" aria-labelledby={`nav-${sec.label}`}>
              <h2 className="nav-section" id={`nav-${sec.label}`}>{sec.label}</h2>
              {sec.items.map((n) => (
                <Link key={n.to} to={n.to} className="nav-link" aria-current={current(n.to) ? 'page' : undefined}>
                  <Icon name={n.icon} /> {n.label}
                </Link>
              ))}
            </section>
          ))}
        </nav>
        <div className="sidebar-foot">
          <div className="user-chip">
            <div className="avatar">{initial(user?.name ?? '')}</div>
            <div>
              <strong>{user?.name}</strong>
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

      <nav className="bottom-nav" aria-label="Navigasi bawah" style={isAdmin ? { gridTemplateColumns: 'repeat(3, 1fr)' } : undefined}>
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
