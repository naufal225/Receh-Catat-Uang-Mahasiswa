import { useEffect, useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { AmountInput, ColorPicker, Empty, Field, Modal, useConfirm, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useFinance } from '../lib/finance';
import { formatRp, walletTypeLabel } from '../lib/format';
import { navigate, useLocation } from '../lib/router';
import type { Wallet, WalletType } from '../lib/types';

// One-tap presets for the wallets Indonesian students most often have.
const presets: { name: string; type: WalletType; color: string }[] = [
  { name: 'BCA', type: 'bank', color: 'c1' },
  { name: 'BRI', type: 'bank', color: 'c7' },
  { name: 'Mandiri', type: 'bank', color: 'c4' },
  { name: 'BNI', type: 'bank', color: 'c2' },
  { name: 'GoPay', type: 'ewallet', color: 'c3' },
  { name: 'DANA', type: 'ewallet', color: 'c1' },
  { name: 'OVO', type: 'ewallet', color: 'c7' },
  { name: 'ShopeePay', type: 'ewallet', color: 'c2' },
  { name: 'Tunai', type: 'cash', color: 'c6' },
];

export default function Wallets() {
  const { wallets, changed, readOnly, loaded, openTx } = useFinance();
  const { query } = useLocation();
  const toast = useToast();
  const confirm = useConfirm();
  const [editing, setEditing] = useState<Wallet | 'new' | null>(null);

  useEffect(() => {
    if (query.get('baru') && !readOnly) {
      setEditing('new');
      navigate('/app/dompet', { replace: true });
    }
  }, [query, readOnly]);

  const total = wallets.reduce((s, w) => s + w.balance, 0);

  async function remove(w: Wallet) {
    const ok = await confirm({
      title: `Hapus ${w.name}?`,
      body: 'Kalau dompet ini sudah punya transaksi, dompet akan diarsipkan agar riwayat dan laporan tetap utuh. Jadwal rutinnya ikut dinonaktifkan.',
      confirmLabel: 'Hapus dompet',
      danger: true,
    });
    if (!ok) return;
    try {
      const r = await api.del<{ archived: boolean }>(`/wallets/${w.id}`);
      toast(r.archived ? `${w.name} diarsipkan (riwayat tetap tersimpan)` : `${w.name} dihapus`);
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Dompet</h1>
          <p>Tempat uangmu disimpan. Total saldo: <strong className="num" style={{ color: 'var(--ink)' }}>{formatRp(total)}</strong></p>
        </div>
        {!readOnly && (
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {wallets.length >= 2 && (
              <button className="btn btn-ghost" onClick={() => openTx({ type: 'transfer' })}><Icon name="transfer" size={18} /> Pindah uang</button>
            )}
            <button className="btn btn-primary" onClick={() => setEditing('new')}><Icon name="plus" size={18} /> Tambah dompet</button>
          </div>
        )}
      </div>

      {loaded && wallets.length === 0 ? (
        <div className="card card-pad">
          <Empty icon="wallet" title="Belum ada dompet" action={!readOnly && <button className="btn btn-primary" onClick={() => setEditing('new')}>Tambah dompet</button>}>
            Tambahkan rekening bank, e-wallet, atau uang tunai beserta saldonya saat ini.
          </Empty>
        </div>
      ) : (
        <div className="grid-cards">
          {wallets.map((w) => (
            <div key={w.id} className={'wallet-card wallet-bg-' + w.color} style={{ minHeight: 150 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', position: 'relative', zIndex: 1 }}>
                <div>
                  <div className="wc-type">{walletTypeLabel[w.type]}</div>
                  <div className="wc-name">{w.name}</div>
                </div>
                {!readOnly && (
                  <div className="row-actions">
                    {wallets.length >= 2 && (
                      <button className="icon-btn" style={{ color: '#fff' }} onClick={() => openTx({ type: 'transfer', fromWalletId: w.id })} aria-label={`Pindahkan uang dari ${w.name}`} title="Pindah uang ke dompet lain"><Icon name="transfer" size={18} /></button>
                    )}
                    <button className="icon-btn" style={{ color: '#fff' }} onClick={() => setEditing(w)} aria-label={`Ubah ${w.name}`}><Icon name="edit" size={18} /></button>
                    <button className="icon-btn" style={{ color: '#fff' }} onClick={() => remove(w)} aria-label={`Hapus ${w.name}`}><Icon name="trash" size={18} /></button>
                  </div>
                )}
              </div>
              <div className="wc-balance" style={{ marginTop: 34, fontSize: 24 }}>{formatRp(w.balance)}</div>
            </div>
          ))}
        </div>
      )}

      <p className="muted" style={{ fontSize: 13.5, marginTop: 18, maxWidth: 620 }}>
        Saldo hanya berubah lewat transaksi dan transfer, supaya riwayat selalu menjelaskan saldo. Top up GoPay dari BCA? Pakai <strong>Pindah uang</strong>: biaya adminnya dipotong dari dompet asal dan tercatat sebagai pengeluaran. Untuk mengoreksi saldo, catat pemasukan atau pengeluaran penyesuaian.
      </p>

      <Modal open={editing !== null} onClose={() => setEditing(null)} title={editing === 'new' ? 'Tambah dompet' : 'Ubah dompet'}>
        {editing !== null && <WalletForm key={editing === 'new' ? 'new' : editing.id} wallet={editing === 'new' ? null : editing} onDone={() => { setEditing(null); changed(); }} />}
      </Modal>
    </>
  );
}

function WalletForm({ wallet, onDone }: { wallet: Wallet | null; onDone: () => void }) {
  const toast = useToast();
  const [name, setName] = useState(wallet?.name ?? '');
  const [type, setType] = useState<WalletType>(wallet?.type ?? 'ewallet');
  const [color, setColor] = useState(wallet?.color ?? 'c1');
  const [balance, setBalance] = useState(0);
  const [error, setError] = useState<{ field?: string; message: string } | null>(null);
  const [saving, setSaving] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!name.trim()) return setError({ field: 'name', message: 'Nama dompet wajib diisi.' });
    setSaving(true);
    try {
      if (wallet) await api.put(`/wallets/${wallet.id}`, { name, type, color });
      else await api.post('/wallets', { name, type, color, balance });
      toast(wallet ? 'Dompet diperbarui' : `Dompet ${name} ditambahkan`);
      onDone();
    } catch (err) {
      const e2 = err as ApiError;
      setError({ field: e2.field, message: e2.message });
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      {!wallet && (
        <div className="field">
          <span className="label">Pilih cepat</span>
          <div className="chips">
            {presets.map((p) => (
              <button key={p.name} type="button" className="chip" aria-pressed={name === p.name} onClick={() => { setName(p.name); setType(p.type); setColor(p.color); }}>
                <span className={'dot swatch-' + p.color} />{p.name}
              </button>
            ))}
          </div>
        </div>
      )}
      <Field label="Nama dompet" htmlFor="w-name" error={error?.field === 'name' || error?.message.includes('nama') ? error.message : undefined}>
        <input id="w-name" className="input" value={name} maxLength={40} onChange={(e) => setName(e.target.value)} placeholder="Contoh: BCA Tabungan" />
      </Field>
      <Field label="Jenis" htmlFor="w-type">
        <select id="w-type" className="select" value={type} onChange={(e) => setType(e.target.value as WalletType)}>
          {Object.entries(walletTypeLabel).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
      </Field>
      {!wallet && (
        <Field label="Saldo saat ini" htmlFor="w-balance" hint="Isi dengan saldo yang ada sekarang. Boleh 0." error={error?.field === 'balance' ? error.message : undefined}>
          <AmountInput id="w-balance" value={balance} onChange={setBalance} />
        </Field>
      )}
      <Field label="Warna">
        <ColorPicker value={color} onChange={setColor} />
      </Field>
      {error && !error.field && !error.message.includes('nama') && <div className="alert alert-danger" role="alert">{error.message}</div>}
      <div className="modal-actions">
        <button type="submit" className="btn btn-primary" disabled={saving}>{saving ? 'Menyimpan…' : 'Simpan'}</button>
      </div>
    </form>
  );
}
