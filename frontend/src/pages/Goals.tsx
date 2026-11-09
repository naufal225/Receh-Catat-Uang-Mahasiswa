// Savings goals: progress ring, what to save per week to make the deadline,
// and deposits/withdrawals (or a linked savings wallet that tracks itself).
import { useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { AmountInput, ColorPicker, Empty, ErrorNote, Field, Link, Modal, Segmented, Skeleton, useConfirm, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useFinance } from '../lib/finance';
import { formatDate, formatRp, todayISO } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { GoalContribution, GoalProgress } from '../lib/types';

/** Progress ring: the number in the middle carries the meaning, not the colour. */
export function GoalRing({ g, size = 76 }: { g: GoalProgress; size?: number }) {
  const r = size / 2 - 6;
  const c = 2 * Math.PI * r;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="goal-ring" aria-hidden="true">
      <circle cx={size / 2} cy={size / 2} r={r} className="gr-track" />
      <circle cx={size / 2} cy={size / 2} r={r} className="gr-fill" style={{ stroke: `var(--${g.color})` }}
        strokeDasharray={c} strokeDashoffset={c * (1 - g.percent / 100)} transform={`rotate(-90 ${size / 2} ${size / 2})`} />
      <text x="50%" y="50%" dominantBaseline="central" textAnchor="middle">{g.achieved ? '✓' : `${g.percent}%`}</text>
    </svg>
  );
}

export function goalPace(g: GoalProgress): string {
  if (g.achieved) return 'Tercapai!';
  if (g.overdue) return 'Tenggat sudah lewat';
  if (g.daysLeft !== null && g.perWeek > 0) return `Sisihkan ${formatRp(g.perWeek)}/minggu · ${g.daysLeft} hari lagi`;
  return `Kurang ${formatRp(g.remaining)}`;
}

export default function Goals() {
  const { version, changed, readOnly, membership, wallets } = useFinance();
  const toast = useToast();
  const confirm = useConfirm();
  const [showArchived, setShowArchived] = useState(false);
  const { data, error, loading, reload } = useFetch<GoalProgress[]>(`/goals?v=${version}${showArchived ? '&archived=1' : ''}`);
  const [editing, setEditing] = useState<GoalProgress | 'new' | null>(null);
  const [saving, setSaving] = useState<{ goal: GoalProgress; dir: 'in' | 'out' } | null>(null);
  const [history, setHistory] = useState<GoalProgress | null>(null);

  const cap = membership?.billingEnabled && membership.limits.maxGoals !== -1 ? membership.limits.maxGoals : null;
  const active = (data ?? []).filter((g) => !g.archivedAt);
  const atCap = cap !== null && active.length >= cap;

  async function archive(g: GoalProgress, archived: boolean) {
    try {
      await api.post(`/goals/${g.id}/archive`, { archived });
      toast(archived ? `${g.name} diarsipkan` : `${g.name} aktif lagi`);
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  async function remove(g: GoalProgress) {
    const ok = await confirm({
      title: 'Hapus target?',
      body: <>Target <strong>{g.name}</strong> dan riwayat setorannya dihapus. {g.manualSaving ? 'Saldo dompetmu tidak berubah, karena setoran hanya catatan.' : `Saldo dompet ${g.walletName} tidak berubah.`}</>,
      confirmLabel: 'Hapus', danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/goals/${g.id}`);
      toast('Target dihapus');
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Target tabungan</h1>
          <p>
            Menabung untuk sesuatu, dengan langkah mingguan yang jelas.
            {cap !== null && <> Paket Gratis: {active.length}/{cap} target aktif. <Link to="/app/langganan" style={{ fontWeight: 650, color: 'var(--ink)' }}>Premium →</Link></>}
          </p>
        </div>
        {!readOnly && (
          <button className="btn btn-primary" onClick={() => setEditing('new')} disabled={atCap} title={atCap ? 'Batas paket Gratis tercapai' : undefined}>
            <Icon name="plus" size={18} /> Buat target
          </button>
        )}
      </div>

      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}

      {!data ? (
        <div className="goal-grid">{[0, 1].map((i) => <Skeleton key={i} h={190} r={18} />)}</div>
      ) : data.length === 0 ? (
        <div className="card card-pad">
          <Empty icon="target" title="Belum ada target" action={!readOnly && <button className="btn btn-primary" onClick={() => setEditing('new')}>Buat target pertama</button>}>
            Contoh: Laptop baru Rp 8.000.000 sebelum semester depan. Receh menghitung berapa yang perlu disisihkan tiap minggu.
          </Empty>
        </div>
      ) : (
        <div className={'goal-grid' + (loading ? ' dim' : '')}>
          {data.map((g) => (
            <article key={g.id} className={'card goal-card' + (g.achieved ? ' achieved' : '') + (g.archivedAt ? ' archived' : '')}>
              <div className="goal-top">
                <GoalRing g={g} />
                <div style={{ minWidth: 0 }}>
                  <h3 className="goal-name">{g.name}</h3>
                  <div className="num goal-figures">{formatRp(g.saved)} <span className="muted">/ {formatRp(g.targetAmount)}</span></div>
                  <div className={'goal-pace' + (g.overdue ? ' expense-text' : g.achieved ? ' income-text' : '')}>{goalPace(g)}</div>
                </div>
              </div>
              <div className="goal-meta">
                {g.deadline && <span><Icon name="clock" size={13} /> {formatDate(g.deadline, false)}</span>}
                <span><Icon name="wallet" size={13} /> {g.manualSaving ? 'Setoran manual' : `Saldo ${g.walletName}`}</span>
                {g.archivedAt && <span className="badge badge-neutral">Diarsipkan</span>}
              </div>
              {!readOnly && (
                <div className="goal-actions">
                  {!g.archivedAt && g.manualSaving && (
                    <>
                      <button className="btn btn-primary btn-sm" onClick={() => setSaving({ goal: g, dir: 'in' })}><Icon name="plus" size={16} /> Setor</button>
                      <button className="btn btn-ghost btn-sm" onClick={() => setSaving({ goal: g, dir: 'out' })} disabled={g.saved <= 0}>Tarik</button>
                      <button className="btn btn-quiet btn-sm" onClick={() => setHistory(g)}>Riwayat</button>
                    </>
                  )}
                  {!g.archivedAt && !g.manualSaving && (
                    <Link to="/app/dompet" className="btn btn-ghost btn-sm"><Icon name="transfer" size={16} /> Pindah uang ke {g.walletName}</Link>
                  )}
                  <div className="goal-icons">
                  {!g.archivedAt && <button className="icon-btn" onClick={() => setEditing(g)} aria-label={`Ubah ${g.name}`}><Icon name="edit" size={18} /></button>}
                  <button className="icon-btn" onClick={() => archive(g, !g.archivedAt)} aria-label={g.archivedAt ? `Aktifkan ${g.name}` : `Arsipkan ${g.name}`} title={g.archivedAt ? 'Aktifkan lagi' : 'Arsipkan'}>
                    <Icon name={g.archivedAt ? 'play' : 'check'} size={18} />
                  </button>
                  <button className="icon-btn" onClick={() => remove(g)} aria-label={`Hapus ${g.name}`}><Icon name="trash" size={18} /></button>
                  </div>
                </div>
              )}
            </article>
          ))}
        </div>
      )}

      <p style={{ marginTop: 16 }}>
        <button className="btn btn-quiet btn-sm" onClick={() => setShowArchived((v) => !v)}>
          {showArchived ? 'Sembunyikan target yang diarsipkan' : 'Tampilkan target yang diarsipkan'}
        </button>
      </p>

      <Modal open={editing !== null} onClose={() => setEditing(null)} title={editing === 'new' ? 'Buat target tabungan' : 'Ubah target'}>
        {editing !== null && <GoalForm goal={editing === 'new' ? null : editing} wallets={wallets} onDone={() => { setEditing(null); changed(); }} />}
      </Modal>
      <Modal open={!!saving} onClose={() => setSaving(null)} title={saving?.dir === 'out' ? 'Tarik dari tabungan' : 'Setor ke tabungan'}>
        {saving && <ContributionForm goal={saving.goal} dir={saving.dir} onDone={() => { setSaving(null); changed(); }} />}
      </Modal>
      <Modal open={!!history} onClose={() => setHistory(null)} title={history ? `Riwayat · ${history.name}` : ''}>
        {history && <ContributionHistory goal={history} onChanged={changed} />}
      </Modal>
    </>
  );
}

function GoalForm({ goal, wallets, onDone }: { goal: GoalProgress | null; wallets: { id: number; name: string; balance: number }[]; onDone: () => void }) {
  const toast = useToast();
  const [name, setName] = useState(goal?.name ?? '');
  const [target, setTarget] = useState(goal?.targetAmount ?? 0);
  const [deadline, setDeadline] = useState(goal?.deadline ?? '');
  const [mode, setMode] = useState<'manual' | 'wallet'>(goal && !goal.manualSaving ? 'wallet' : 'manual');
  const [walletId, setWalletId] = useState(goal?.walletId ?? wallets[0]?.id ?? 0);
  const [color, setColor] = useState(goal?.color ?? 'c3');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    if (!name.trim()) next.name = 'Beri nama target.';
    if (!target) next.targetAmount = 'Masukkan nominal target.';
    setErrors(next);
    if (Object.keys(next).length) return;
    setBusy(true);
    const body = { name, targetAmount: target, deadline, color, walletId: mode === 'wallet' ? walletId : 0 };
    try {
      if (goal) await api.put(`/goals/${goal.id}`, body);
      else await api.post('/goals', body);
      toast(goal ? 'Target diperbarui' : `Target ${name} dibuat`);
      onDone();
    } catch (err) {
      const e2 = err as ApiError;
      setCode(e2.code);
      setErrors({ [e2.field || 'form']: e2.message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      <Field label="Nama target" htmlFor="g-name" error={errors.name}>
        <input id="g-name" className="input" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} placeholder="Contoh: Laptop baru" autoFocus />
      </Field>
      <Field label="Nominal target" htmlFor="g-target" error={errors.targetAmount}>
        <AmountInput id="g-target" value={target} onChange={setTarget} invalid={!!errors.targetAmount} />
      </Field>
      <Field label="Tenggat (opsional)" htmlFor="g-deadline" error={errors.deadline} hint="Dengan tenggat, Receh menghitung setoran per minggu.">
        <input id="g-deadline" className="input" type="date" min={todayISO()} value={deadline} onChange={(e) => setDeadline(e.target.value)} />
      </Field>
      <Field label="Cara menabung" error={errors.walletId}>
        <Segmented label="Cara menabung" value={mode} onChange={setMode}
          options={[{ value: 'manual', label: 'Setoran manual' }, { value: 'wallet', label: 'Dompet khusus' }]} />
      </Field>
      {mode === 'wallet' && (
        <Field label="Progres mengikuti saldo dompet" hint="Pakai dompet khusus tabungan; setor dengan fitur Transfer.">
          <div className="chips" role="group" aria-label="Dompet">
            {wallets.map((w) => (
              <button key={w.id} type="button" className="chip" aria-pressed={walletId === w.id} onClick={() => setWalletId(w.id)}>
                {w.name} <small className="num">{formatRp(w.balance)}</small>
              </button>
            ))}
          </div>
        </Field>
      )}
      <Field label="Warna"><ColorPicker value={color} onChange={setColor} /></Field>
      {errors.form && (
        <div className="alert alert-danger" role="alert">
          <div>{errors.form}{code === 'PLAN_LIMIT' && <> <Link to="/app/langganan" style={{ fontWeight: 700, color: 'inherit' }}>Lihat Premium →</Link></>}</div>
        </div>
      )}
      <div className="modal-actions"><button type="submit" className="btn btn-primary" disabled={busy}>{busy ? 'Menyimpan…' : 'Simpan'}</button></div>
    </form>
  );
}

function ContributionForm({ goal, dir, onDone }: { goal: GoalProgress; dir: 'in' | 'out'; onDone: () => void }) {
  const toast = useToast();
  const [amount, setAmount] = useState(dir === 'in' && goal.perWeek > 0 ? goal.perWeek : 0);
  const [note, setNote] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!amount) return setError('Masukkan nominalnya.');
    setBusy(true);
    try {
      const g = await api.post<GoalProgress>(`/goals/${goal.id}/contributions`, { amount: dir === 'in' ? amount : -amount, note });
      toast(g.achieved && !goal.achieved ? `Target ${goal.name} tercapai!` : dir === 'in' ? `${formatRp(amount)} disetor` : `${formatRp(amount)} ditarik`);
      onDone();
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      <p style={{ color: 'var(--ink-2)' }}>
        <strong>{goal.name}</strong>: {formatRp(goal.saved)} dari {formatRp(goal.targetAmount)}.
        {dir === 'in' && ' Ini catatan alokasi; saldo dompetmu tidak berubah.'}
      </p>
      <Field label="Nominal" htmlFor="ct-amount" error={error} hint={dir === 'in' && goal.perWeek > 0 ? `Saran: ${formatRp(goal.perWeek)} per minggu agar tepat waktu.` : undefined}>
        <AmountInput id="ct-amount" value={amount} onChange={setAmount} invalid={!!error} autoFocus />
      </Field>
      <Field label="Catatan (opsional)" htmlFor="ct-note">
        <input id="ct-note" className="input" value={note} maxLength={80} onChange={(e) => setNote(e.target.value)} placeholder={dir === 'in' ? 'Contoh: Sisa uang saku minggu ini' : 'Contoh: Keperluan mendadak'} />
      </Field>
      <div className="modal-actions"><button type="submit" className="btn btn-primary" disabled={busy}>{busy ? 'Menyimpan…' : dir === 'in' ? 'Setor' : 'Tarik'}</button></div>
    </form>
  );
}

function ContributionHistory({ goal, onChanged }: { goal: GoalProgress; onChanged: () => void }) {
  const toast = useToast();
  const { data, reload } = useFetch<GoalContribution[]>(`/goals/${goal.id}/contributions`);
  async function remove(c: GoalContribution) {
    try {
      await api.del(`/goals/${goal.id}/contributions/${c.id}`);
      toast('Catatan dihapus');
      reload();
      onChanged();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }
  if (!data) return <Skeleton h={120} />;
  if (data.length === 0) return <Empty icon="list" title="Belum ada setoran" />;
  return (
    <div className="ent-list">
      {data.map((c) => (
        <div key={c.id} className="ent-row">
          <div>
            <strong className={c.amount > 0 ? 'income-text' : 'expense-text'}>{formatRp(c.amount, true)}</strong>
            <span className="sub">{formatDate(c.occurredOn)}{c.note && ` · ${c.note}`}</span>
          </div>
          <button className="icon-btn" onClick={() => remove(c)} aria-label="Hapus catatan"><Icon name="trash" size={16} /></button>
        </div>
      ))}
    </div>
  );
}
