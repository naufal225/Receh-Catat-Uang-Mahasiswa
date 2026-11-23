// Layout for signed-in pages: sidebar on desktop, top bar + bottom nav with a
// central "+" on mobile. Also hosts the theme toggle and the frozen banner.
import { useEffect, useState, type ReactNode } from 'react';
import { useAuth } from '../lib/auth';
import { brand } from '../brand';
import { useApp } from '../lib/app';
import { useStoredState } from '../lib/hooks';
import { navigate, useLocation } from '../lib/router';
import { modules } from '../modules';
import type { NavItem, PrimaryAction } from '../modules/types';
import { Icon, Logo } from './Icon';
import { NotificationBell } from './Notifications';
import { initial, Link, Modal } from './ui';

const coreUserNav: NavItem[] = [
  { to: '/app/notifikasi', label: 'Notifikasi', icon: 'bell' },
  { to: '/app/keamanan', label: 'Perangkat & keamanan', icon: 'device' },
  { to: '/app/langganan', label: 'Langganan', icon: 'sparkle' },
];

const moduleNav = modules.flatMap((m) => m.nav);
// Without a module home page, the core one gets the first nav slot.
const userNav: NavItem[] = [
  ...(moduleNav.some((n) => n.to === '/app') ? [] : [{ to: '/app', label: 'Beranda', icon: 'home', short: 'Beranda' } as NavItem]),
  ...moduleNav,
  ...coreUserNav,
];

// The first module that declares a primary action owns the big "+" button.
const primary: PrimaryAction | undefined = modules.find((m) => m.primary)?.primary;
const noAction = () => ({ run: () => {}, disabled: true });

const adminNav: NavItem[] = [
  { to: '/admin', label: 'Pengguna', icon: 'users', short: 'Pengguna' },
  { to: '/admin/audit', label: 'Audit log', icon: 'shield', short: 'Audit' },
  { to: '/admin/kode', label: 'Kode undangan', icon: 'sparkle', short: 'Kode' },
  { to: '/admin/keamanan', label: 'Perangkat saya', icon: 'device', short: 'Perangkat' },
];

function ThemeToggle() {
  const [theme, setTheme] = useStoredState<'light' | 'dark' | ''>(`${brand.storagePrefix}-theme`, '');
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
  const { readOnly, membership } = useApp();
  // Static per build, so the hook order never changes between renders.
  const action = (primary?.use ?? noAction)();
  const openAction = action.run;
  const actionDisabled = readOnly || !!action.disabled;
  const { path } = useLocation();
  const isAdmin = user?.role === 'admin';
  // "Langganan" only exists where something is sold (billing on).
  const nav = isAdmin ? adminNav : userNav.filter((n) => n.to !== '/app/langganan' || membership?.billingEnabled);
  const isPremium = !isAdmin && membership?.plan === 'premium' && membership.billingEnabled;
  const current = (to: string) => (to === '/app' || to === '/admin' ? path === to : path.startsWith(to));

  // The primary action's shortcut works from anywhere, unless the user is typing.
  const shortcut = primary?.shortcut;
  useEffect(() => {
    if (isAdmin || actionDisabled || !shortcut) return;
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (e.key.toLowerCase() !== shortcut || e.metaKey || e.ctrlKey || e.altKey) return;
      if (t.closest('input, textarea, select, [contenteditable], dialog')) return;
      e.preventDefault();
      openAction();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isAdmin, actionDisabled, openAction, shortcut]);

  async function signOut() {
    await logout();
    navigate('/masuk', { replace: true });
  }

  const mobileItems = nav.filter((n) => n.short);
  // Everything not in the bottom bar lives in the mobile menu sheet.
  const menuItems = nav.filter((n) => !n.short);
  const [menuOpen, setMenuOpen] = useState(false);
  useEffect(() => setMenuOpen(false), [path]);

  return (
    <div className="shell">
      <aside className="sidebar" aria-label="Navigasi utama">
        <Link to={isAdmin ? '/admin' : '/app'} className="brand">
          <Logo /> {brand.name}
        </Link>
        {!isAdmin && primary && (
          <button type="button" className="btn btn-accent" style={{ marginBottom: 14 }} onClick={openAction} disabled={actionDisabled}
            aria-keyshortcuts={primary.shortcut?.toUpperCase()} title={primary.shortcut ? `Pintasan keyboard: ${primary.shortcut.toUpperCase()}` : undefined}>
            <Icon name={primary.icon} size={18} /> {primary.label}
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
            <div style={{ display: 'flex', gap: 2 }}>
              <ThemeToggle />
              {!isAdmin && <NotificationBell />}
            </div>
            <button type="button" className="btn btn-quiet btn-sm" onClick={signOut}>
              <Icon name="logout" size={18} /> Keluar
            </button>
          </div>
        </div>
      </aside>

      <header className="mobile-top">
        <Link to={isAdmin ? '/admin' : '/app'} className="brand">
          <Logo size={26} /> {brand.name}
        </Link>
        <div style={{ display: 'flex', gap: 2 }}>
          {!isAdmin && <NotificationBell />}
          <ThemeToggle />
          <button type="button" className="icon-btn" onClick={() => setMenuOpen(true)} aria-label="Menu" aria-haspopup="dialog">
            <Icon name="menu" />
          </button>
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

      <Modal open={menuOpen} onClose={() => setMenuOpen(false)} title="Menu">
        <div className="menu-sheet">
          <div className="user-chip" style={{ padding: '0 0 12px' }}>
            <div className="avatar">{initial(user?.name ?? '')}</div>
            <div>
              <strong>{user?.name}{isPremium && <span className="badge badge-accent premium-tag"><Icon name="sparkle" size={11} /> Premium</span>}</strong>
              <span>{user?.email}</span>
            </div>
          </div>
          {menuItems.map((n) => (
            <Link key={n.to} to={n.to} className="nav-link" aria-current={current(n.to) ? 'page' : undefined}>
              <Icon name={n.icon} /> {n.label}
            </Link>
          ))}
          <button type="button" className="nav-link" onClick={signOut}>
            <Icon name="logout" /> Keluar
          </button>
        </div>
      </Modal>

      <nav className="bottom-nav" aria-label="Navigasi bawah" style={isAdmin || !primary ? { gridTemplateColumns: `repeat(${mobileItems.length}, 1fr)` } : undefined}>
        {mobileItems.slice(0, 2).map((n) => (
          <Link key={n.to} to={n.to} aria-current={current(n.to) ? 'page' : undefined}>
            <Icon name={n.icon} /> {n.short}
          </Link>
        ))}
        {!isAdmin && primary && (
          <button type="button" onClick={openAction} disabled={actionDisabled} aria-label={primary.label}>
            <span className="fab"><Icon name={primary.icon} size={26} /></span>
            {primary.short}
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
