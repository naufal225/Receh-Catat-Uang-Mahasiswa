// "Masuk dengan Google". A plain link, not a fetch: the browser must actually
// travel to Google's account picker and back. The server keeps the client
// secret and the PKCE verifier; the browser only ever sees Google's URL.
import { useEffect, useState } from 'react';
import { api } from '../lib/api';

let providers: Promise<{ google: boolean }> | null = null;

/** Whether the server has Google sign-in configured, asked once per tab. */
export function useGoogleEnabled(): boolean {
  const [enabled, setEnabled] = useState(false);
  useEffect(() => {
    providers ??= api.get<{ google: boolean }>('/auth/providers').catch(() => ({ google: false }));
    let live = true;
    providers.then((p) => live && setEnabled(p.google));
    return () => {
      live = false;
    };
  }, []);
  return enabled;
}

/** Google's four-colour "G", as its branding guidelines ask buttons to use. */
function GoogleMark() {
  return (
    <svg width="18" height="18" viewBox="0 0 48 48" aria-hidden="true">
      <path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z" />
      <path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z" />
      <path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z" />
      <path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z" />
    </svg>
  );
}

export function GoogleButton({ label = 'Masuk dengan Google' }: { label?: string }) {
  return (
    <a className="btn btn-google btn-block" href="/api/auth/google/start">
      <GoogleMark />
      {label}
    </a>
  );
}

/** "atau" between the Google button and the email form. */
export function OrDivider() {
  return (
    <div className="or-divider" role="separator">
      <span>atau dengan email</span>
    </div>
  );
}

/**
 * Reads, then deletes, the one-minute notice cookie the server sets with a
 * suspension reason. The reason never travels in the URL, so a crafted link
 * cannot make this page show someone else's text.
 */
export function takeServerNotice(): string | null {
  const m = document.cookie.match(/(?:^|;\s*)receh_notice=([^;]*)/);
  if (!m) return null;
  document.cookie = 'receh_notice=; Max-Age=0; Path=/; SameSite=Lax';
  try {
    return decodeURIComponent(m[1].replace(/\+/g, ' '));
  } catch {
    return null;
  }
}

/** Explains the codes the server appends as ?oauth= when Google sign-in fails. */
export function oauthMessage(code: string | null, detail: string | null): { text: string; tone: 'danger' | 'info' } | null {
  switch (code) {
    case null:
    case '':
      return null;
    case 'OAUTH_CANCELLED':
      return { text: 'Masuk dengan Google dibatalkan. Kamu bisa coba lagi atau pakai email.', tone: 'info' };
    case 'OAUTH_EMAIL_UNVERIFIED':
      return { text: 'Email akun Google itu belum diverifikasi Google. Verifikasi dulu di Google, atau daftar dengan email.', tone: 'danger' };
    case 'OAUTH_ADMIN':
      return { text: 'Akun admin hanya bisa masuk dengan email dan kata sandi.', tone: 'danger' };
    case 'OAUTH_DISABLED':
      return { text: 'Masuk dengan Google belum diaktifkan di server ini.', tone: 'info' };
    case 'ACCOUNT_SUSPENDED':
      return { text: detail || 'Akun ini ditangguhkan oleh admin.', tone: 'danger' };
    case 'TOO_MANY':
      return { text: 'Terlalu banyak pendaftaran dari jaringan ini. Coba lagi nanti.', tone: 'danger' };
    default:
      return { text: 'Masuk dengan Google gagal. Coba lagi sebentar lagi.', tone: 'danger' };
  }
}
