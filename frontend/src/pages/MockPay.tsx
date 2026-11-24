// The fake gateway's checkout page (BILLING_PROVIDER=mock). It exists so the
// whole payment flow can be tried and tested without a real payment account.
import { useState } from 'react';
import { useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { navigate, useLocation } from '../lib/router';

export default function MockPay() {
  const { query } = useLocation();
  const ref = query.get('ref') ?? '';
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  async function finish(outcome: 'paid' | 'failed' | 'expired') {
    setBusy(true);
    try {
      await api.post(`/billing/mock/${encodeURIComponent(ref)}/${outcome}`);
      navigate('/app/langganan?status=return');
    } catch (e) {
      toast((e as ApiError).message, 'error');
      setBusy(false);
    }
  }

  return (
    <div className="card card-pad" style={{ maxWidth: 480, display: 'grid', gap: 14 }}>
      <span className="badge badge-warn" style={{ justifySelf: 'start' }}>Gateway tiruan · hanya untuk pengembangan</span>
      <h1 style={{ fontSize: 22 }}>Halaman pembayaran tiruan</h1>
      <p className="muted">Tidak ada uang sungguhan. Pilih hasil pembayaran untuk mencoba alurnya.</p>
      <p className="mono" style={{ fontSize: 13, wordBreak: 'break-all' }}>{ref || 'referensi tidak ada'}</p>
      <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
        <button type="button" className="btn btn-primary" disabled={busy || !ref} onClick={() => finish('paid')}>Bayar sekarang</button>
        <button type="button" className="btn btn-ghost" disabled={busy || !ref} onClick={() => finish('failed')}>Gagal</button>
        <button type="button" className="btn btn-ghost" disabled={busy || !ref} onClick={() => finish('expired')}>Kedaluwarsa</button>
      </div>
    </div>
  );
}
