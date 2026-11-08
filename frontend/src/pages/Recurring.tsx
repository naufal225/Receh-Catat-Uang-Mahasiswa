// Recurring income/expense rules plus a plain-language monthly plan: how much
// is committed every month, and what is left once the routine items run.
import { useMemo, useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { AmountInput, Empty, ErrorNote, Field, Modal, Segmented, Skeleton, useConfirm, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useFinance } from '../lib/finance';
import { formatDate, formatRp, frequencyLabel, todayISO } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { Frequency, Recurring as Rule, TxType } from '../lib/types';

const perMonth = (r: Rule) => {
  switch (r.frequency) {
    case 'daily': return r.amount * 30;
    case 'weekly': return Math.round((r.amount * 52) / 12);
    case 'yearly': return Math.round(r.amount / 12);
    default: return r.amount;
  }
};

export default function Recurring() {
  const { version, changed, readOnly } = useFinance();
  const toast = useToast();
  const confirm = useConfirm();
  const { data, error, reload } = useFetch<Rule[]>(`/recurring?v=${version}`);
  const [editing, setEditing] = useState<Rule | 'new' | null>(null);

  const plan = useMemo(() => {
    const active = (data ?? []).filter((r) => r.active);
    const income = active.filter((r) => r.type === 'income').reduce((s, r) => s + perMonth(r), 0);
    const expense = active.filter((r) => r.type === 'expense').reduce((s, r) => s + perMonth(r), 0);
    return { income, expense };
  }, [data]);

  async function toggle(r: Rule) {
    try {
      await api.post(`/recurring/${r.id}/active`, { active: !r.active });
      toast(r.active ? `${r.name} dijeda` : `${r.name} aktif lagi`);
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }
  async function remove(r: Rule) {
    const ok = await confirm({ title: `Hapus jadwal ${r.name}?`, body: 'Transaksi yang sudah tercatat tetap ada di riwayat. Hanya jadwal ke depan yang dihapus.', confirmLabel: 'Hapus jadwal', danger: true });
    if (!ok) return;
    try {
      await api.del(`/recurring/${r.id}`);
      toast('Jadwal dihapus');
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Rutin & rencana</h1>
          <p>Tagihan dan pemasukan berulang dicatat otomatis saat jatuh tempo, misalnya listrik atau Netflix tiap bulan.</p>
        </div>
        {!readOnly && <button className="btn btn-primary" onClick={() => setEditing('new')}><Icon name="plus" size={18} /> Jadwal baru</button>}
      </div>

      <div className="stat-row">
        <div className="card stat"><span className="label">Pemasukan rutin / bulan</span><strong className="income-text">{formatRp(plan.income)}</strong></div>
        <div className="card stat"><span className="label">Pengeluaran rutin / bulan</span><strong className="expense-text">{formatRp(plan.expense)}</strong></div>
        <div className="card stat"><span className="label">Sisa setelah yang rutin</span><strong>{formatRp(plan.income - plan.expense, true)}</strong></div>
      </div>

      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}
      <div className="card" style={{ overflow: 'hidden' }}>
        {!data ? (
          <div style={{ padding: 18, display: 'grid', gap: 14 }}>{[0, 1, 2].map((i) => <Skeleton key={i} h={44} />)}</div>
        ) : data.length === 0 ? (
          <Empty icon="repeat" title="Belum ada jadwal rutin" action={!readOnly && <button className="btn btn-primary" onClick={() => setEditing('new')}>Buat jadwal</button>}>
            Contoh: Listrik kos Rp 50.000 setiap bulan, Spotify setiap bulan, atau uang saku setiap minggu.
          </Empty>
        ) : (
          <div className="list-card">
            {data.map((r) => (
              <div key={r.id} className="list-item" style={{ opacity: r.active ? 1 : 0.6, flexWrap: 'wrap' }}>
                <div className={'tx-icon swatch-' + r.categoryColor}><Icon name="repeat" size={18} /></div>
                <div className="grow">
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                    <strong>{r.name}</strong>
                    {!r.active && <span className="badge badge-neutral"><Icon name="pause" size={12} /> Dijeda</span>}
                    {r.lastError && r.active && <span className="badge badge-warn"><Icon name="alert" size={12} /> Saldo kurang</span>}
                  </div>
                  <div className="tx-meta">
                    {frequencyLabel[r.frequency]} · {r.categoryName} · {r.walletName}
                    {r.active && <> · Berikutnya <strong style={{ color: 'var(--ink-2)' }}>{formatDate(r.nextDue)}</strong></>}
                  </div>
                  {r.lastError && r.active && <div className="tx-meta" style={{ color: 'var(--warn)' }}>{r.lastError}</div>}
                </div>
                <div className={'tx-amount ' + (r.type === 'income' ? 'income-text' : 'expense-text')}>{formatRp(r.type === 'income' ? r.amount : -r.amount, true)}</div>
                {!readOnly && (
                  <div className="row-actions">
                    <button className="icon-btn" onClick={() => toggle(r)} aria-label={r.active ? `Jeda ${r.name}` : `Aktifkan ${r.name}`} title={r.active ? 'Jeda' : 'Aktifkan'}><Icon name={r.active ? 'pause' : 'play'} size={18} /></button>
                    <button className="icon-btn" onClick={() => setEditing(r)} aria-label={`Ubah ${r.name}`}><Icon name="edit" size={18} /></button>
                    <button className="icon-btn" onClick={() => remove(r)} aria-label={`Hapus ${r.name}`}><Icon name="trash" size={18} /></button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
      <p className="muted" style={{ fontSize: 13.5, marginTop: 14, maxWidth: 680 }}>
        Kalau saldo dompet tidak cukup saat jatuh tempo, transaksi menunggu dan dicoba lagi otomatis. Menjeda lalu mengaktifkan lagi tidak menagih periode yang terlewat.
      </p>

      <Modal open={editing !== null} onClose={() => setEditing(null)} title={editing === 'new' ? 'Jadwal rutin baru' : 'Ubah jadwal rutin'}>
        {editing !== null && <RuleForm key={editing === 'new' ? 'new' : editing.id} rule={editing === 'new' ? null : editing} onDone={() => { setEditing(null); changed(); }} />}
      </Modal>
    </>
  );
}

function RuleForm({ rule, onDone }: { rule: Rule | null; onDone: () => void }) {
  const { wallets, categories } = useFinance();
  const toast = useToast();
  const [type, setType] = useState<TxType>(rule?.type ?? 'expense');
  const [amount, setAmount] = useState(rule?.amount ?? 0);
  const [name, setName] = useState(rule?.name ?? '');
  const [frequency, setFrequency] = useState<Frequency>(rule?.frequency ?? 'monthly');
  const [walletId, setWalletId] = useState(rule?.walletId ?? wallets[0]?.id ?? 0);
  const [categoryId, setCategoryId] = useState(rule?.categoryId ?? 0);
  const [startOn, setStartOn] = useState(rule?.nextDue ?? todayISO());
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const cats = categories.filter((c) => c.type === type);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    if (!amount) next.amount = 'Masukkan nominalnya.';
    if (!name.trim()) next.name = 'Beri nama jadwalnya.';
    if (!walletId) next.walletId = 'Pilih dompet.';
    if (!cats.some((c) => c.id === categoryId)) next.categoryId = 'Pilih kategori.';
    setErrors(next);
    if (Object.keys(next).length) return;
    setSaving(true);
    try {
      const body = { type, amount, name, frequency, walletId, categoryId, startOn };
      const saved = rule ? await api.put<Rule>(`/recurring/${rule.id}`, body) : await api.post<Rule>('/recurring', body);
      if (saved.lastError) toast('Jadwal tersimpan, tetapi saldo belum cukup untuk transaksi hari ini.', 'error');
      else if (startOn === todayISO() && !rule) toast(`Jadwal tersimpan, transaksi hari ini sudah dicatat`);
      else toast('Jadwal tersimpan');
      onDone();
    } catch (err) {
      const e2 = err as ApiError;
      setErrors(e2.field ? { [e2.field]: e2.message } : { form: e2.message });
    } finally {
      setSaving(false);
    }
  }

  if (wallets.length === 0) {
    return <Empty icon="wallet" title="Tambahkan dompet dulu">Jadwal rutin butuh dompet sumber atau tujuan.</Empty>;
  }

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      <Segmented big label="Jenis" value={type} onChange={(t) => { setType(t); setCategoryId(0); }}
        options={[{ value: 'expense', label: 'Pengeluaran', tone: 'expense' }, { value: 'income', label: 'Pemasukan', tone: 'income' }]} />
      <Field label="Nominal" htmlFor="r-amount" error={errors.amount}>
        <AmountInput id="r-amount" value={amount} onChange={setAmount} invalid={!!errors.amount} autoFocus />
      </Field>
      <Field label="Nama" htmlFor="r-name" error={errors.name}>
        <input id="r-name" className="input" value={name} maxLength={80} onChange={(e) => setName(e.target.value)} placeholder={type === 'expense' ? 'Contoh: Netflix' : 'Contoh: Uang saku bulanan'} />
      </Field>
      <div className="form-row">
        <Field label="Seberapa sering" htmlFor="r-freq">
          <select id="r-freq" className="select" value={frequency} onChange={(e) => setFrequency(e.target.value as Frequency)}>
            {Object.entries(frequencyLabel).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </select>
        </Field>
        <Field label={rule ? 'Jatuh tempo berikutnya' : 'Mulai tanggal'} htmlFor="r-start" error={errors.startOn}
          hint={frequency === 'monthly' ? 'Tanggal 29–31 menyesuaikan di bulan yang lebih pendek.' : undefined}>
          <input id="r-start" type="date" className="input" value={startOn} min={todayISO()} onChange={(e) => setStartOn(e.target.value)} />
        </Field>
      </div>
      <Field label="Dompet" error={errors.walletId}>
        <div className="chips">
          {wallets.map((w) => (
            <button key={w.id} type="button" className="chip" aria-pressed={walletId === w.id} onClick={() => setWalletId(w.id)}><span className={'dot swatch-' + w.color} />{w.name}</button>
          ))}
        </div>
      </Field>
      <Field label="Kategori" error={errors.categoryId}>
        <div className="chips">
          {cats.map((c) => (
            <button key={c.id} type="button" className="chip" aria-pressed={categoryId === c.id} onClick={() => setCategoryId(c.id)}><span className={'dot swatch-' + c.color} />{c.name}</button>
          ))}
        </div>
      </Field>
      {errors.form && <div className="alert alert-danger" role="alert">{errors.form}</div>}
      <div className="modal-actions">
        <button type="submit" className="btn btn-primary" disabled={saving}>{saving ? 'Menyimpan…' : 'Simpan jadwal'}</button>
      </div>
    </form>
  );
}
