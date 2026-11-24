// Public pricing page (/harga), built from the server's catalog. With billing
// off (the personal edition) it says everything is free.
import { useState } from 'react';
import { brand, pick } from '../brand';
import { Icon, Logo } from '../components/Icon';
import { Link, Segmented, Skeleton } from '../components/ui';
import { useAuth } from '../lib/auth';
import { formatRp } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { CatalogView, LimitDef } from '../lib/types';
import { yearlySaving } from './Subscription';

function featureText(d: LimitDef, v: number): string {
  if (d.feature) return v !== 0 ? pick(d.label) : '';
  return `${pick(d.label)}: ${v === -1 ? 'tanpa batas' : v}`;
}

export default function Pricing() {
  const { user } = useAuth();
  const { data, error } = useFetch<CatalogView>('/billing/catalog');
  const prices = data?.catalog.prices.filter((p) => p.plan === 'premium') ?? [];
  const saving = yearlySaving(prices);
  const [interval, setInterval] = useState<'month' | 'year'>('year');
  const chosen = prices.find((p) => p.interval === interval) ?? prices[0];
  const planName = pick(data?.catalog.planName) || 'Pro';
  const cta = user ? (user.role === 'admin' ? '/admin' : '/app/langganan') : '/daftar';

  return (
    <div className="landing">
      <header className="l-nav">
        <Link to="/" className="brand"><Logo /> {brand.name}</Link>
        <nav aria-label="Navigasi">
          {user
            ? <Link to={user.role === 'admin' ? '/admin' : '/app'} className="btn btn-primary">Buka aplikasi</Link>
            : (<>
              <Link to="/masuk" className="btn btn-ghost">Masuk</Link>
              <Link to="/daftar" className="btn btn-primary">Daftar</Link>
            </>)}
        </nav>
      </header>

      <main className="pricing">
        <h1>Harga</h1>
        {error && !data && <p className="muted">Harga belum bisa dimuat. Coba lagi sebentar.</p>}
        {!data && !error && <Skeleton h={220} />}
        {data && !data.enabled && (
          <p className="muted">{brand.name} edisi ini gratis: semua fitur terbuka tanpa batas.</p>
        )}
        {data?.enabled && chosen && (
          <>
            <p className="muted">Mulai gratis. Naik ke {planName} kalau butuh lebih. Bayar per periode, tanpa tagihan otomatis.</p>
            {prices.length > 1 && (
              <div style={{ margin: '18px 0' }}>
                <Segmented label="Lama langganan" value={interval} onChange={setInterval}
                  options={prices.map((p) => ({ value: p.interval, label: p.interval === 'year' ? `Tahunan${saving > 0 ? ` · hemat ${saving} bulan` : ''}` : 'Bulanan' }))} />
              </div>
            )}
            <div className="pricing-grid">
              <section className="card card-pad pricing-card">
                <h2>Gratis</h2>
                <div className="price-amount"><strong>{formatRp(0)}</strong></div>
                <ul className="pricing-list">
                  {data.defs.map((d) => {
                    const t = featureText(d, d.free);
                    return <li key={d.key} className={t ? '' : 'off'}><Icon name={t ? 'check' : 'close'} size={16} /> {t || pick(d.label)}</li>;
                  })}
                </ul>
                <Link to={user ? '/app' : '/daftar'} className="btn btn-ghost">{user ? 'Buka aplikasi' : 'Daftar gratis'}</Link>
              </section>
              <section className="card card-pad pricing-card is-featured">
                <h2>{planName}</h2>
                <div className="price-amount">
                  <strong>{formatRp(chosen.amount)}</strong>
                  <span className="muted"> / {chosen.interval === 'year' ? 'tahun' : 'bulan'}</span>
                </div>
                <ul className="pricing-list">
                  {data.defs.map((d) => <li key={d.key}><Icon name="check" size={16} /> {featureText(d, d.premium) || pick(d.label)}</li>)}
                </ul>
                <Link to={cta} className="btn btn-primary">{user ? 'Berlangganan' : `Daftar dan pilih ${planName}`}</Link>
              </section>
            </div>
          </>
        )}
      </main>
    </div>
  );
}
