import type { Frequency, WalletType } from './types';

const rupiah = new Intl.NumberFormat('id-ID');

/** "Rp 1.250.000". Signed variants put the sign before "Rp". */
export function formatRp(v: number, sign = false): string {
  const abs = 'Rp ' + rupiah.format(Math.abs(v));
  if (sign) return (v < 0 ? '− ' : '+ ') + abs;
  return v < 0 ? '− ' + abs : abs;
}

/** Short form for chart axes: "1,2 jt", "350 rb". */
export function formatShort(v: number): string {
  const a = Math.abs(v);
  if (a >= 1e9) return trim(v / 1e9) + ' M';
  if (a >= 1e6) return trim(v / 1e6) + ' jt';
  if (a >= 1e3) return trim(v / 1e3) + ' rb';
  return String(v);
}
function trim(n: number) {
  return n.toLocaleString('id-ID', { maximumFractionDigits: 1 });
}

/** Digits typed into an amount field, shown with thousand separators. */
export function digitsOnly(s: string): number {
  const d = s.replace(/\D/g, '').slice(0, 13);
  return d ? Number(d) : 0;
}
export function groupDigits(n: number): string {
  return n ? rupiah.format(n) : '';
}

export const monthsLong = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember'];
const monthsShort = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];
const days = ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu'];

function parseISODate(iso: string): Date {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(y, m - 1, d);
}

/** Local calendar date as YYYY-MM-DD. */
export function todayISO(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export function shiftISO(iso: string, days: number): string {
  const d = parseISODate(iso);
  d.setDate(d.getDate() + days);
  return todayISO(d);
}

/** "7 Nov 2026", or "Hari ini" / "Kemarin" for the two most common cases. */
export function formatDate(iso: string, relative = true): string {
  if (relative) {
    const today = todayISO();
    if (iso === today) return 'Hari ini';
    if (iso === shiftISO(today, -1)) return 'Kemarin';
  }
  const d = parseISODate(iso);
  return `${d.getDate()} ${monthsShort[d.getMonth()]} ${d.getFullYear()}`;
}

export function formatDayHeading(iso: string): string {
  const d = parseISODate(iso);
  const base = `${days[d.getDay()]}, ${d.getDate()} ${monthsLong[d.getMonth()]} ${d.getFullYear()}`;
  const rel = formatDate(iso);
  return rel === 'Hari ini' || rel === 'Kemarin' ? `${rel} · ${base}` : base;
}

export function formatDateTime(isoTs: string): string {
  const d = new Date(isoTs);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getDate()} ${monthsShort[d.getMonth()]} ${d.getFullYear()}, ${p(d.getHours())}.${p(d.getMinutes())}`;
}

export function timeAgo(isoTs: string): string {
  const s = Math.max(0, (Date.now() - new Date(isoTs).getTime()) / 1000);
  if (s < 60) return 'Baru saja';
  if (s < 3600) return `${Math.floor(s / 60)} menit lalu`;
  if (s < 86400) return `${Math.floor(s / 3600)} jam lalu`;
  if (s < 86400 * 7) return `${Math.floor(s / 86400)} hari lalu`;
  return formatDateTime(isoTs);
}

export function monthLabel(ym: string): string {
  const [y, m] = ym.split('-').map(Number);
  return `${monthsLong[m - 1]} ${y}`;
}

export const frequencyLabel: Record<Frequency, string> = {
  daily: 'Setiap hari',
  weekly: 'Setiap minggu',
  monthly: 'Setiap bulan',
  yearly: 'Setiap tahun',
};

export const walletTypeLabel: Record<WalletType, string> = {
  bank: 'Rekening bank',
  ewallet: 'E-wallet',
  cash: 'Tunai',
  other: 'Lainnya',
};
