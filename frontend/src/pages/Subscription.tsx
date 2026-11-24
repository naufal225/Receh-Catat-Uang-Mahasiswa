// Plan status, buying or renewing Pro, payment history, and redeeming a code.
// On the personal edition (billing off) it simply says everything is unlocked.
import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { Field, Segmented, Skeleton, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { brand, pick } from '../brand';
import { useApp } from '../lib/app';
import { formatDateTime, formatRp } from '../lib/format';
import { useFetch } from '../lib/hooks';
import { useLocation } from '../lib/router';
import type { BillingOverview, CatalogView, LimitDef, Membership, Payment, PaymentStatus, PricePoint, SubState } from '../lib/types';

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

export const paymentStatusLabel: Record<PaymentStatus, { label: string; cls: string }> = {
  pending: { label: 'Menunggu bayar', cls: 'badge-warn' },
  paid: { label: 'Lunas', cls: 'badge-income' },
  expired: { label: 'Kedaluwarsa', cls: 'badge-neutral' },
  failed: { label: 'Gagal', cls: 'badge-danger' },
};

const subLabel: Record<SubState, string> = {
  none: 'Belum berlangganan',
  active: 'Aktif',
  ending: 'Aktif, tidak diperpanjang',
  grace: 'Masa tenggang',
  expired: 'Berakhir',
};

function cell(d: LimitDef, v: number) {
  if (d.feature) {
    return v !== 0 ? <span className="income-text"><Icon name="check" size={18} /><span className="sr-only">Ya</span></span> : <span className="muted">—</span>;
  }
  return limitText(v);
}

const intervalWord = (p: PricePoint) => (p.interval === 'year' ? 'tahun' : 'bulan');

/** How many months of the monthly price the yearly one saves; 0 if none. */
export function yearlySaving(prices: PricePoint[]): number {
  const m = prices.find((p) => p.interval === 'month');
  const y = prices.find((p) => p.interval === 'year');
  if (!m || !y) return 0;
  return Math.max(0, Math.floor((m.amount * 12 - y.amount) / m.amount));
}

export default function Subscription() {
  const { membership: m, changed, version, readOnly } = useApp();
  const catalog = useFetch<CatalogView>(m?.billingEnabled ? '/billing/catalog' : null);
  const overview = useFetch<BillingOverview>(m?.billingEnabled ? `/billing?v=${version}` : null);
  // Back from the gateway the webhook has usually already changed the plan,
  // but the plan shown at the top was loaded before: refresh it once.
  const returned = useLocation().query.get('status') === 'return';
  useEffect(() => {
    if (returned) changed();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [returned]);
  if (!m) return <Skeleton h={200} />;
  const premium = m.plan === 'premium';
  const planName = pick(catalog.data?.catalog.planName) || 'Pro';
  const ov = overview.data;
  const selling = !!ov?.enabled && !!catalog.data?.enabled;

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Langganan</h1>
          <p>{m.billingEnabled ? 'Paket yang kamu pakai dan apa saja isinya.' : `Edisi pribadi ${brand.name}.`}</p>
        </div>
      </div>

      <div className={'card card-pad plan-card' + (premium ? ' is-premium' : '')}>
        <div className="plan-head">
          <span className={'badge ' + (premium ? 'badge-accent' : 'badge-neutral')}>
            {premium && <Icon name="sparkle" size={12} />} {premium ? planName : 'Gratis'}
          </span>
          {m.billingEnabled && premium && (
            <span className="muted">
              {/* A paid entitlement also covers the grace days; show the paid period's own end. */}
              {m.source === 'paid' && ov?.subscription ? `Sampai ${formatDateTime(ov.subscription.periodEnd)}` : premiumUntil(m.endsAt)}
            </span>
          )}
        </div>
        <h2>{!m.billingEnabled ? `Semua fitur ${planName} aktif` : premium ? `Kamu memakai ${planName}` : 'Kamu memakai paket Gratis'}</h2>
        <p className="muted">
          {!m.billingEnabled
            ? 'Edisi pribadi tidak menjual apa pun: setiap akun otomatis mendapat semua fitur tanpa batas.'
            : premium
              ? sourceLabel(m) || `Terima kasih sudah mendukung ${brand.name}.`
              : `Semua fitur dasar tetap gratis. ${planName} membuka batas yang tertera di tabel.`}
        </p>
      </div>

      {m.billingEnabled && ov && <SubscriptionStatus ov={ov} planName={planName} catalog={catalog.data} onChange={() => { overview.reload(); changed(); }} readOnly={readOnly} />}

      {selling && ov.isOwner && catalog.data && (
        <Plans catalog={catalog.data} ov={ov} planName={planName} readOnly={readOnly} onChange={() => { overview.reload(); changed(); }} />
      )}

      <div className="card" style={{ overflow: 'hidden', marginTop: 20 }}>
        <div className="table-scroll">
          <table className="table plan-table">
            <thead>
              <tr><th>Fitur</th>{m.billingEnabled && <th>Gratis</th>}<th>{planName}</th></tr>
            </thead>
            <tbody>
              {m.defs.map((d) => (
                <tr key={d.key}>
                  <td>
                    {pick(d.label)}
                    {!d.feature && m.usage[d.key] !== undefined && m.billingEnabled && !premium && (
                      <span className="sub">Terpakai {m.usage[d.key]} dari {limitText(d.free)}</span>
                    )}
                  </td>
                  {m.billingEnabled && <td>{cell(d, d.free)}</td>}
                  <td>{cell(d, d.premium)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {m.billingEnabled && !m.isOwner && (
        <p className="muted" style={{ marginTop: 14 }}>
          Tim ini memakai paket milik owner-nya. Untuk mengubah paket, minta owner atau pindah ke workspace pribadimu.
        </p>
      )}

      {selling && ov.isOwner && ov.payments.length > 0 && <History payments={ov.payments} catalog={catalog.data} />}

      {m.billingEnabled && m.isOwner && <RedeemForm onDone={changed} />}
    </>
  );
}

/** Current subscription, a pending checkout, and the way back from the gateway. */
function SubscriptionStatus({ ov, planName, catalog, onChange, readOnly }: {
  ov: BillingOverview; planName: string; catalog: CatalogView | null; onChange: () => void; readOnly: boolean;
}) {
  const toast = useToast();
  const { query } = useLocation();
  const returned = query.get('status') === 'return';
  const [busy, setBusy] = useState(false);
  const sub = ov.subscription;
  const pending = ov.pending;
  const priceLabel = (id: string) => pick(catalog?.catalog.prices.find((p) => p.id === id)?.label) || id;

  // Coming back from the gateway: the webhook usually got there first, but
  // ask a few times in case it is a moment late.
  const pendingId = ov.isOwner && returned ? pending?.id : undefined;
  useEffect(() => {
    if (!pendingId) return;
    let tries = 0;
    const timer = setInterval(async () => {
      tries++;
      try {
        const p = await api.post<Payment>(`/billing/payments/${pendingId}/sync`);
        if (p.status !== 'pending') {
          clearInterval(timer);
          onChange();
        }
      } catch {
        /* try again on the next tick */
      }
      if (tries >= 6) clearInterval(timer);
    }, 3000);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingId]);

  async function run(fn: () => Promise<unknown>, done?: string) {
    setBusy(true);
    try {
      await fn();
      if (done) toast(done);
      onChange();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    } finally {
      setBusy(false);
    }
  }

  if (!sub && !pending && !returned) return null;
  return (
    <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
      {returned && !pending && sub && (sub.state === 'active' || sub.state === 'ending') && (
        <div className="alert alert-info" role="status">Pembayaran diterima. Terima kasih! {planName} aktif sampai {formatDateTime(sub.periodEnd)}.</div>
      )}
      {returned && pending && (
        <div className="alert alert-info" role="status" aria-busy="true">Menunggu konfirmasi pembayaran dari penyedia… ini biasanya hanya beberapa detik.</div>
      )}

      {sub && (
        <div className="card card-pad" style={{ display: 'grid', gap: 8 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            <strong>{priceLabel(sub.priceId)}</strong>
            <span className={'badge ' + (sub.state === 'active' ? 'badge-income' : sub.state === 'expired' ? 'badge-neutral' : 'badge-warn')}>{subLabel[sub.state]}</span>
          </div>
          <p className="muted" style={{ fontSize: 14 }}>
            {sub.state === 'grace'
              ? <>Periode berakhir {formatDateTime(sub.periodEnd)}. Fitur {planName} tetap terbuka sampai {formatDateTime(sub.graceEndsAt)}; perpanjang agar tidak turun ke Gratis.</>
              : sub.state === 'expired'
                ? <>Berakhir {formatDateTime(sub.periodEnd)}. Workspace kembali ke Gratis; datamu tetap aman.</>
                : sub.state === 'ending'
                  ? <>Aktif sampai {formatDateTime(sub.periodEnd)}, setelah itu kembali ke Gratis.</>
                  : <>Aktif sampai {formatDateTime(sub.periodEnd)}. Kamu akan diingatkan untuk memperpanjang.</>}
          </p>
          {ov.isOwner && (sub.state === 'active' || sub.state === 'ending') && (
            <div>
              {sub.cancelAtPeriodEnd
                ? <button type="button" className="btn btn-ghost btn-sm" disabled={busy || readOnly} onClick={() => run(() => api.post('/billing/subscription/resume'), 'Pengingat perpanjangan aktif lagi')}>Lanjutkan perpanjangan</button>
                : <button type="button" className="btn btn-ghost btn-sm" disabled={busy || readOnly} onClick={() => run(() => api.post('/billing/subscription/stop'), 'Tidak diperpanjang. Paket tetap aktif sampai periode habis.')}>Berhenti memperpanjang</button>}
            </div>
          )}
        </div>
      )}

      {ov.isOwner && pending && (
        <div className="card card-pad" style={{ display: 'grid', gap: 10 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            <strong>{priceLabel(pending.priceId)} · {formatRp(pending.amount)}</strong>
            <span className="badge badge-warn">Menunggu bayar</span>
          </div>
          <p className="muted" style={{ fontSize: 14 }}>Bayar sebelum {formatDateTime(pending.expiresAt)}. Setelah membayar, paket aktif otomatis.</p>
          <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
            {pending.checkoutUrl && <a className="btn btn-primary" href={pending.checkoutUrl}>Lanjutkan pembayaran</a>}
            <button type="button" className="btn btn-ghost" disabled={busy} onClick={() => run(() => api.post(`/billing/payments/${pending.id}/sync`), 'Status diperbarui')}>Saya sudah bayar</button>
            <button type="button" className="btn btn-ghost" disabled={busy || readOnly} onClick={() => run(() => api.post(`/billing/payments/${pending.id}/cancel`), 'Pembayaran dibatalkan')}>Batalkan</button>
          </div>
        </div>
      )}
    </div>
  );
}

/** Buying or renewing: pick monthly or yearly, then go to the gateway's page. */
function Plans({ catalog, ov, planName, readOnly }: {
  catalog: CatalogView; ov: BillingOverview; planName: string; readOnly: boolean; onChange: () => void;
}) {
  const toast = useToast();
  const prices = catalog.catalog.prices.filter((p) => p.plan === 'premium');
  const saving = yearlySaving(prices);
  const [interval, setInterval] = useState<'month' | 'year'>(prices.some((p) => p.interval === 'year') ? 'year' : 'month');
  const chosen = prices.find((p) => p.interval === interval) ?? prices[0];
  const [busy, setBusy] = useState(false);
  const renewing = !!ov.subscription && ov.subscription.state !== 'none';

  const options = useMemo(
    () => prices.map((p) => ({ value: p.interval, label: p.interval === 'year' ? `Tahunan${saving > 0 ? ` · hemat ${saving} bulan` : ''}` : 'Bulanan' })),
    [prices, saving],
  );

  if (!chosen) return null;

  async function buy(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const p = await api.post<Payment>('/billing/checkout', { priceId: chosen.id });
      if (p.checkoutUrl) window.location.assign(p.checkoutUrl);
    } catch (err) {
      toast((err as ApiError).message, 'error');
      setBusy(false);
    }
  }

  return (
    <form className="card card-pad price-card" style={{ marginTop: 20 }} onSubmit={buy}>
      <div>
        <h2 style={{ fontSize: 18, fontFamily: 'var(--font-sans)', fontWeight: 750 }}>{renewing ? `Perpanjang ${planName}` : `Berlangganan ${planName}`}</h2>
        <p className="muted" style={{ fontSize: 14 }}>
          Sekali bayar per periode, tanpa tagihan otomatis. Perpanjang lebih awal tidak menghilangkan sisa harimu.
        </p>
      </div>
      {options.length > 1 && <Segmented label="Lama langganan" value={interval} onChange={setInterval} options={options} />}
      <div className="price-amount">
        <strong>{formatRp(chosen.amount)}</strong>
        <span className="muted"> / {intervalWord(chosen)}</span>
      </div>
      <div>
        <button type="submit" className="btn btn-primary" disabled={busy || readOnly || !!ov.pending && ov.pending.priceId === chosen.id}>
          {busy ? 'Membuka pembayaran…' : renewing ? 'Perpanjang' : 'Berlangganan'}
        </button>
        {ov.pending && ov.pending.priceId !== chosen.id && <span className="muted" style={{ fontSize: 13, marginLeft: 10 }}>Pembayaran yang menunggu akan diganti.</span>}
      </div>
    </form>
  );
}

function History({ payments, catalog }: { payments: Payment[]; catalog: CatalogView | null }) {
  const label = (id: string) => pick(catalog?.catalog.prices.find((p) => p.id === id)?.label) || id;
  return (
    <>
      <h2 style={{ fontSize: 18, fontFamily: 'var(--font-sans)', fontWeight: 750, margin: '24px 0 10px' }}>Riwayat pembayaran</h2>
      <div className="card" style={{ overflow: 'hidden' }}>
        <div className="table-scroll">
          <table className="table">
            <thead><tr><th>Tanggal</th><th>Paket</th><th className="num">Jumlah</th><th>Status</th></tr></thead>
            <tbody>
              {payments.map((p) => (
                <tr key={p.id}>
                  <td>{formatDateTime(p.paidAt ?? p.createdAt)}</td>
                  <td>{label(p.priceId)}{p.periodEnd && <span className="sub">Sampai {formatDateTime(p.periodEnd)}</span>}</td>
                  <td className="num">{formatRp(p.amount)}</td>
                  <td><span className={'badge ' + paymentStatusLabel[p.status].cls}>{paymentStatusLabel[p.status].label}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

function RedeemForm({ onDone }: { onDone: () => void }) {
  const toast = useToast();
  const { readOnly } = useApp();
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
      toast(`Paket aktif · ${premiumUntil(m.endsAt)}`);
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
        <p className="muted" style={{ fontSize: 14 }}>Tukarkan untuk mendapat paket Pro gratis.</p>
      </div>
      <Field label="Kode" htmlFor="promo-code" error={error}>
        <input id="promo-code" className="input mono" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())}
          placeholder="Contoh: KELUARGA-2026" autoComplete="off" spellCheck={false} maxLength={32} aria-invalid={!!error || undefined} />
      </Field>
      <div><button type="submit" className="btn btn-primary" disabled={busy || readOnly}>{busy ? 'Memeriksa…' : 'Tukarkan kode'}</button></div>
    </form>
  );
}
