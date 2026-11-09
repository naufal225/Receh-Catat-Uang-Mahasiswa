// Invitation codes that grant premium, e.g. one code for the whole family.
import { useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { Empty, ErrorNote, Field, Modal, Segmented, Skeleton, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { formatDateTime, todayISO } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { PromoCode } from '../lib/types';

export default function AdminPromos() {
  const toast = useToast();
  const { data, error, loading, reload } = useFetch<{ billingEnabled: boolean; items: PromoCode[] }>('/admin/promo-codes');
  const [creating, setCreating] = useState(false);

  async function toggle(p: PromoCode) {
    try {
      await api.post(`/admin/promo-codes/${p.id}/active`, { active: !p.active });
      toast(p.active ? `Kode ${p.code} dinonaktifkan` : `Kode ${p.code} aktif lagi`);
      reload();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  async function copy(code: string) {
    try {
      await navigator.clipboard.writeText(code);
      toast(`${code} disalin`);
    } catch {
      toast('Tidak bisa menyalin otomatis. Salin kodenya manual.', 'error');
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Kode undangan</h1>
          <p>Bagikan kode ke keluarga atau teman untuk Premium gratis. Setiap akun hanya bisa memakai satu kode sekali.</p>
        </div>
        <button className="btn btn-primary" onClick={() => setCreating(true)}><Icon name="plus" size={18} /> Buat kode</button>
      </div>

      {data && !data.billingEnabled && (
        <div className="alert alert-info" style={{ marginBottom: 16 }}>
          Server ini berjalan sebagai <strong>edisi pribadi</strong> (<code>BILLING_ENABLED=false</code>): semua akun sudah Premium, jadi kode tidak diperlukan.
        </div>
      )}
      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}

      <div className={'card' + (loading && data ? ' dim' : '')} style={{ overflow: 'hidden' }}>
        {!data ? (
          <div style={{ padding: 18, display: 'grid', gap: 12 }}>{[0, 1].map((i) => <Skeleton key={i} h={42} />)}</div>
        ) : data.items.length === 0 ? (
          <Empty icon="sparkle" title="Belum ada kode" action={<button className="btn btn-primary" onClick={() => setCreating(true)}>Buat kode pertama</button>}>
            Contoh: <span className="mono">KELUARGA-2026</span>, kuota 5, Premium selamanya.
          </Empty>
        ) : (
          <div className="table-scroll">
            <table className="table">
              <thead><tr><th>Kode</th><th>Premium</th><th>Terpakai</th><th>Berlaku</th><th style={{ textAlign: 'right' }}>Aksi</th></tr></thead>
              <tbody>
                {data.items.map((p) => {
                  const expired = p.expiresAt !== null && new Date(p.expiresAt) <= new Date();
                  const full = p.used >= p.maxUses;
                  return (
                    <tr key={p.id} style={!p.active || expired || full ? { opacity: 0.6 } : undefined}>
                      <td>
                        <button className="promo-code" onClick={() => copy(p.code)} title="Salin kode"><span className="mono">{p.code}</span> <Icon name="copy" size={14} /></button>
                        {p.note && <span className="sub">{p.note}</span>}
                      </td>
                      <td>{p.durationDays ? `${p.durationDays} hari` : 'Selamanya'}</td>
                      <td className="num">
                        {p.used} / {p.maxUses}
                        <div className="meter" aria-hidden="true"><span style={{ width: `${Math.min(100, (p.used / p.maxUses) * 100)}%` }} /></div>
                      </td>
                      <td>
                        {!p.active ? <span className="badge badge-neutral">Nonaktif</span>
                          : expired ? <span className="badge badge-warn">Kedaluwarsa</span>
                          : full ? <span className="badge badge-warn">Kuota habis</span>
                          : <span className="badge badge-income">Aktif</span>}
                        <span className="sub">{p.expiresAt ? 'Sampai ' + formatDateTime(new Date(new Date(p.expiresAt).getTime() - 60_000).toISOString()) : 'Tanpa batas waktu'}</span>
                      </td>
                      <td style={{ textAlign: 'right' }}>
                        <button className="btn btn-ghost btn-sm" onClick={() => toggle(p)}>{p.active ? 'Nonaktifkan' : 'Aktifkan'}</button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <Modal open={creating} onClose={() => setCreating(false)} title="Buat kode undangan">
        {creating && <PromoForm onDone={(code) => { setCreating(false); reload(); toast(`Kode ${code} dibuat`); }} />}
      </Modal>
    </>
  );
}

function PromoForm({ onDone }: { onDone: (code: string) => void }) {
  const [code, setCode] = useState('');
  const [forever, setForever] = useState<'forever' | 'days'>('forever');
  const [days, setDays] = useState(30);
  const [maxUses, setMaxUses] = useState(5);
  const [expiresOn, setExpiresOn] = useState('');
  const [note, setNote] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const p = await api.post<PromoCode>('/admin/promo-codes', {
        code, maxUses, note, expiresOn, durationDays: forever === 'forever' ? null : days,
      });
      onDone(p.code);
    } catch (err) {
      const e2 = err as ApiError;
      setErrors({ [e2.field || 'form']: e2.message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      <Field label="Kode" htmlFor="pc-code" error={errors.code} hint="Kosongkan untuk dibuatkan acak, mis. RECEH-7KQ2M9XP.">
        <input id="pc-code" className="input mono" value={code} maxLength={32} onChange={(e) => setCode(e.target.value.toUpperCase())} placeholder="KELUARGA-2026" autoFocus autoComplete="off" />
      </Field>
      <Field label="Lama Premium">
        <Segmented label="Lama Premium" value={forever} onChange={setForever}
          options={[{ value: 'forever', label: 'Selamanya' }, { value: 'days', label: 'Sekian hari' }]} />
      </Field>
      {forever === 'days' && (
        <Field label="Jumlah hari" htmlFor="pc-days" error={errors.durationDays}>
          <input id="pc-days" className="input" type="number" min={1} max={3650} value={days} onChange={(e) => setDays(Number(e.target.value))} />
        </Field>
      )}
      <div className="form-row">
        <Field label="Kuota pemakaian" htmlFor="pc-max" error={errors.maxUses} hint="Berapa akun yang bisa memakai.">
          <input id="pc-max" className="input" type="number" min={1} max={10000} value={maxUses} onChange={(e) => setMaxUses(Number(e.target.value))} />
        </Field>
        <Field label="Bisa ditukar sampai (opsional)" htmlFor="pc-exp" error={errors.expiresOn}>
          <input id="pc-exp" className="input" type="date" min={todayISO()} value={expiresOn} onChange={(e) => setExpiresOn(e.target.value)} />
        </Field>
      </div>
      <Field label="Catatan (opsional)" htmlFor="pc-note" error={errors.note} hint="Tampil ke penerima, mis. &quot;Keluarga&quot;.">
        <input id="pc-note" className="input" value={note} maxLength={200} onChange={(e) => setNote(e.target.value)} />
      </Field>
      {errors.form && <div className="alert alert-danger">{errors.form}</div>}
      <div className="modal-actions"><button type="submit" className="btn btn-primary" disabled={busy}>{busy ? 'Membuat…' : 'Buat kode'}</button></div>
    </form>
  );
}
