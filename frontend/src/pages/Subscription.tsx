// Plan status, what each plan includes, and redeeming a code. On the personal
// edition (billing off) it simply says everything is unlocked.
import { useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { Field, Skeleton, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useFinance } from '../lib/finance';
import { formatDateTime } from '../lib/format';
import type { Limits, Membership } from '../lib/types';

const limitText = (n: number) => (n === -1 ? 'Tanpa batas' : String(n));

/** The last moment premium is active, shown as the minute before it ends. */
export function premiumUntil(endsAt: string | null): string {
  if (!endsAt) return 'Selamanya';
  return 'Sampai ' + formatDateTime(new Date(new Date(endsAt).getTime() - 60_000).toISOString());
}

export function sourceLabel(m: Pick<Membership, 'source' | 'note'>): string {
  switch (m.source) {
    case 'comp':
      return 'Hadiah dari admin' + (m.note ? ` · ${m.note}` : '');
    case 'code':
      return 'Dari kode undangan' + (m.note ? ` · ${m.note}` : '');
    case 'paid':
      return 'Langganan berbayar';
    default:
      return '';
  }
}

const rows: { label: string; key: keyof Limits; usage?: 'wallets' | 'budgets' | 'goals' }[] = [
  { label: 'Dompet aktif', key: 'maxWallets', usage: 'wallets' },
  { label: 'Anggaran per kategori', key: 'maxBudgets', usage: 'budgets' },
  { label: 'Target tabungan', key: 'maxGoals', usage: 'goals' },
  { label: 'Impor mutasi bank (CSV)', key: 'csvImport' },
];

function cell(v: number | boolean) {
  if (typeof v === 'boolean') {
    return v ? <span className="income-text"><Icon name="check" size={18} /><span className="sr-only">Ya</span></span> : <span className="muted">—</span>;
  }
  return limitText(v);
}

export default function Subscription() {
  const { membership: m, changed } = useFinance();
  if (!m) return <Skeleton h={200} />;
  const premiumLimits: Limits = { maxWallets: -1, maxBudgets: -1, maxGoals: -1, csvImport: true };
  const premium = m.plan === 'premium';

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Langganan</h1>
          <p>{m.billingEnabled ? 'Paket yang kamu pakai dan apa saja isinya.' : 'Edisi pribadi Receh.'}</p>
        </div>
      </div>

      <div className={'card card-pad plan-card' + (premium ? ' is-premium' : '')}>
        <div className="plan-head">
          <span className={'badge ' + (premium ? 'badge-accent' : 'badge-neutral')}>
            {premium && <Icon name="sparkle" size={12} />} {premium ? 'Premium' : 'Gratis'}
          </span>
          {m.billingEnabled && premium && <span className="muted">{premiumUntil(m.endsAt)}</span>}
        </div>
        <h2>{!m.billingEnabled ? 'Semua fitur Premium aktif' : premium ? 'Kamu memakai Premium' : 'Kamu memakai paket Gratis'}</h2>
        <p className="muted">
          {!m.billingEnabled
            ? 'Edisi pribadi tidak menjual apa pun: setiap akun otomatis mendapat semua fitur tanpa batas.'
            : premium
              ? sourceLabel(m) || 'Terima kasih sudah mendukung Receh.'
              : 'Semua fitur dasar tetap gratis. Premium membuka batas dompet, anggaran, target, dan impor mutasi.'}
        </p>
      </div>

      <div className="card" style={{ overflow: 'hidden', marginTop: 20 }}>
        <div className="table-scroll">
          <table className="table plan-table">
            <thead>
              <tr><th>Fitur</th>{m.billingEnabled && <th>Gratis</th>}<th>Premium</th></tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.key}>
                  <td>
                    {r.label}
                    {r.usage && m.billingEnabled && !premium && (
                      <span className="sub">Terpakai {m.usage[r.usage]} dari {limitText(m.freeLimits[r.key] as number)}</span>
                    )}
                  </td>
                  {m.billingEnabled && <td>{cell(m.freeLimits[r.key])}</td>}
                  <td>{cell(premiumLimits[r.key])}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {m.billingEnabled && (
        <>
          <RedeemForm onDone={changed} />
          <p className="muted" style={{ fontSize: 13.5, marginTop: 14 }}>
            Pembayaran online (QRIS, virtual account, e-wallet) segera hadir.
          </p>
        </>
      )}
    </>
  );
}

function RedeemForm({ onDone }: { onDone: () => void }) {
  const toast = useToast();
  const { readOnly } = useFinance();
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!code.trim()) return setError('Masukkan kodenya.');
    setBusy(true);
    setError('');
    try {
      const m = await api.post<Membership>('/membership/redeem', { code });
      toast(`Premium aktif · ${premiumUntil(m.endsAt)}`);
      setCode('');
      onDone();
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card card-pad form-grid" style={{ maxWidth: 520, marginTop: 20 }} onSubmit={submit} noValidate>
      <div>
        <h2 style={{ fontSize: 18, fontFamily: 'var(--font-sans)', fontWeight: 750 }}>Punya kode undangan?</h2>
        <p className="muted" style={{ fontSize: 14 }}>Tukarkan untuk mendapat Premium gratis.</p>
      </div>
      <Field label="Kode" htmlFor="promo-code" error={error}>
        <input id="promo-code" className="input mono" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())}
          placeholder="Contoh: KELUARGA-2026" autoComplete="off" spellCheck={false} maxLength={32} aria-invalid={!!error || undefined} />
      </Field>
      <div><button type="submit" className="btn btn-primary" disabled={busy || readOnly}>{busy ? 'Memeriksa…' : 'Tukarkan kode'}</button></div>
    </form>
  );
}
