// Monthly budgets per expense category: how much is left, what is close to the
// limit, and what went over. Spending is computed from history by the server.
import { useMemo, useState, type FormEvent } from 'react';
import { BudgetBar } from '../components/BudgetBar';
import { Icon } from '../components/Icon';
import { AmountInput, Empty, ErrorNote, Field, Link, Modal, Skeleton, useConfirm, useToast } from '../components/ui';
import { api, ApiError, qs } from '../lib/api';
import { useFinance } from '../lib/finance';
import { formatRp } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { BudgetMonth, BudgetStatus } from '../lib/types';

function shiftMonth(ym: string, delta: number): string {
  const [y, m] = ym.split('-').map(Number);
  const d = new Date(Date.UTC(y, m - 1 + delta, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`;
}

export default function Budgets() {
  const { version, changed, readOnly, membership } = useFinance();
  const toast = useToast();
  const confirm = useConfirm();
  const [month, setMonth] = useState('');
  const { data, error, loading, reload } = useFetch<BudgetMonth>(`/budgets${qs({ month, v: version })}`);
  const [editing, setEditing] = useState<BudgetStatus | 'new' | null>(null);

  const cap = membership?.billingEnabled && membership.limits.maxBudgets !== -1 ? membership.limits.maxBudgets : null;
  const atCap = cap !== null && (data?.items.length ?? 0) >= cap;
  // Most urgent first: over, then warning, then by share used.
  const items = useMemo(() => {
    const rank = { over: 0, warning: 1, ok: 2 } as const;
    return [...(data?.items ?? [])].sort((a, b) => rank[a.level] - rank[b.level] || b.percent - a.percent);
  }, [data]);

  async function remove(b: BudgetStatus) {
    const ok = await confirm({ title: 'Hapus anggaran?', body: <>Anggaran <strong>{b.categoryName}</strong> dihapus. Transaksinya tidak terpengaruh.</>, confirmLabel: 'Hapus', danger: true });
    if (!ok) return;
    try {
      await api.del(`/budgets/${b.id}`);
      toast('Anggaran dihapus');
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  const total = data ? { budget: data.totalBudget, spent: data.totalSpent } : null;
  const pct = total && total.budget ? Math.round((total.spent / total.budget) * 100) : 0;

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Anggaran</h1>
          <p>
            Batas belanja per kategori setiap bulan.
            {cap !== null && <> Paket Gratis: {data?.items.length ?? 0}/{cap} anggaran. <Link to="/app/langganan" style={{ fontWeight: 650, color: 'var(--ink)' }}>Premium →</Link></>}
          </p>
        </div>
        {!readOnly && (
          <button className="btn btn-primary" onClick={() => setEditing('new')} disabled={atCap} title={atCap ? 'Batas paket Gratis tercapai' : undefined}>
            <Icon name="plus" size={18} /> Buat anggaran
          </button>
        )}
      </div>

      <div className="period-nav" style={{ marginBottom: 16 }}>
        <button className="icon-btn" onClick={() => setMonth(shiftMonth(data?.month ?? '', -1))} disabled={!data} aria-label="Bulan sebelumnya"><Icon name="left" /></button>
        <strong>{data?.title ?? '…'}</strong>
        <button className="icon-btn" onClick={() => setMonth(shiftMonth(data!.month, 1))} disabled={!data || data.isCurrent} aria-label="Bulan berikutnya"><Icon name="right" /></button>
        {data && !data.isCurrent && <button className="btn btn-quiet btn-sm" onClick={() => setMonth('')}>Bulan ini</button>}
      </div>

      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}

      {!data ? (
        <div className="card card-pad" style={{ display: 'grid', gap: 16 }}>{[0, 1, 2].map((i) => <Skeleton key={i} h={48} />)}</div>
      ) : data.items.length === 0 ? (
        <div className="card card-pad">
          <Empty icon="pie" title="Belum ada anggaran" action={!readOnly && <button className="btn btn-primary" onClick={() => setEditing('new')}>Buat anggaran pertama</button>}>
            Contoh: Makan Rp 1.200.000 per bulan. Kamu akan melihat sisa anggaran setiap kali mencatat pengeluaran.
          </Empty>
        </div>
      ) : (
        <div className={loading ? 'dim' : undefined}>
          <div className="card card-pad budget-summary">
            <div>
              <span className="label">Terpakai {data.isCurrent ? 'bulan ini' : ''}</span>
              <strong className="num">{formatRp(data.totalSpent)}</strong>
              <span className="muted"> dari {formatRp(data.totalBudget)}</span>
            </div>
            <div>
              <span className="label">{data.totalBudget - data.totalSpent >= 0 ? 'Sisa' : 'Lebih'}</span>
              <strong className={'num ' + (data.totalBudget - data.totalSpent >= 0 ? '' : 'expense-text')}>{formatRp(Math.abs(data.totalBudget - data.totalSpent))}</strong>
              {data.isCurrent && data.totalBudget > data.totalSpent && (
                <span className="muted"> · ±{formatRp(Math.floor((data.totalBudget - data.totalSpent) / Math.max(1, data.daysLeft)))}/hari selama {data.daysLeft} hari</span>
              )}
            </div>
            <div className="bb-track big" aria-hidden="true"><span style={{ width: `${Math.min(100, pct)}%` }} className={pct > 100 ? 'over' : pct >= 80 ? 'warning' : ''} /></div>
            {data.unbudgeted > 0 && <p className="muted" style={{ fontSize: 13 }}>Pengeluaran lain tanpa anggaran bulan ini: {formatRp(data.unbudgeted)}</p>}
          </div>

          <div className="card budget-list">
            {items.map((b) => (
              <div key={b.id} className="budget-row">
                <BudgetBar b={b} />
                {!readOnly && (
                  <div className="row-actions">
                    <button className="icon-btn" onClick={() => setEditing(b)} aria-label={`Ubah anggaran ${b.categoryName}`}><Icon name="edit" size={18} /></button>
                    <button className="icon-btn" onClick={() => remove(b)} aria-label={`Hapus anggaran ${b.categoryName}`}><Icon name="trash" size={18} /></button>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      <Modal open={editing !== null} onClose={() => setEditing(null)} title={editing === 'new' ? 'Buat anggaran' : 'Ubah anggaran'}>
        {editing !== null && (
          <BudgetForm budget={editing === 'new' ? null : editing} taken={data?.items.map((b) => b.categoryId) ?? []}
            onDone={() => { setEditing(null); changed(); }} />
        )}
      </Modal>
    </>
  );
}

function BudgetForm({ budget, taken, onDone }: { budget: BudgetStatus | null; taken: number[]; onDone: () => void }) {
  const { categories } = useFinance();
  const toast = useToast();
  const options = categories.filter((c) => c.type === 'expense' && (!taken.includes(c.id) || c.id === budget?.categoryId));
  const [categoryId, setCategoryId] = useState(budget?.categoryId ?? options[0]?.id ?? 0);
  const [amount, setAmount] = useState(budget?.amount ?? 0);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!amount) return setErrors({ amount: 'Masukkan nominal anggaran.' });
    setBusy(true);
    try {
      if (budget) await api.put(`/budgets/${budget.id}`, { amount });
      else await api.post('/budgets', { categoryId, amount });
      toast(budget ? 'Anggaran diperbarui' : 'Anggaran dibuat');
      onDone();
    } catch (err) {
      const e2 = err as ApiError;
      setCode(e2.code);
      setErrors({ [e2.field || 'form']: e2.message });
    } finally {
      setBusy(false);
    }
  }

  if (!budget && options.length === 0) {
    return <Empty icon="tag" title="Semua kategori sudah punya anggaran">Tambahkan kategori pengeluaran baru di halaman Kategori.</Empty>;
  }

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      {budget ? (
        <p style={{ color: 'var(--ink-2)' }}>Kategori <strong>{budget.categoryName}</strong></p>
      ) : (
        <Field label="Kategori" error={errors.categoryId}>
          <div className="chips" role="group" aria-label="Kategori">
            {options.map((c) => (
              <button key={c.id} type="button" className="chip" aria-pressed={categoryId === c.id} onClick={() => setCategoryId(c.id)}>
                <span className={'dot swatch-' + c.color} />{c.name}
              </button>
            ))}
          </div>
        </Field>
      )}
      <Field label="Batas per bulan" htmlFor="bg-amount" error={errors.amount} hint="Berlaku setiap bulan; terhitung dari pengeluaran kategori ini.">
        <AmountInput id="bg-amount" value={amount} onChange={setAmount} invalid={!!errors.amount} autoFocus />
      </Field>
      {errors.form && (
        <div className="alert alert-danger" role="alert">
          <div>{errors.form}{code === 'PLAN_LIMIT' && <> <Link to="/app/langganan" style={{ fontWeight: 700, color: 'inherit' }}>Lihat Premium →</Link></>}</div>
        </div>
      )}
      <div className="modal-actions"><button type="submit" className="btn btn-primary" disabled={busy}>{busy ? 'Menyimpan…' : 'Simpan'}</button></div>
    </form>
  );
}
