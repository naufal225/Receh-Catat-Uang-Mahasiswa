// Moving money between one's own wallets, e.g. top up GoPay from BCA.
//
// The fee is paid by the source on top of the amount, as banks do: top up
// Rp100.000 with a Rp1.000 fee and BCA drops by Rp101.000 while GoPay gains
// Rp100.000. Fees differ per route (same bank, BI-FAST, e-wallet top up), so
// instead of a fee table the form prefills whatever this route cost last time.
import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { api, ApiError, qs } from '../lib/api';
import { useFinance } from '../lib/finance';
import { formatRp, groupDigits, digitsOnly, shiftISO, todayISO } from '../lib/format';
import type { Transfer, Wallet } from '../lib/types';
import { Icon } from './Icon';
import { AmountInput, Empty, Field, Link, Skeleton, useConfirm, useToast } from './ui';

type Errors = Partial<Record<'fromWalletId' | 'toWalletId' | 'amount' | 'fee' | 'note' | 'occurredOn' | 'form', string>>;

/** Common Indonesian fees, as one-tap presets. The field stays editable. */
const FEE_PRESETS = [
  { value: 0, label: 'Gratis' },
  { value: 1_000, label: '1.000' },
  { value: 2_500, label: '2.500' },
  { value: 6_500, label: '6.500' },
];

/** Balance change per wallet, mirroring the server (sign -1 undoes a transfer). */
function effects(t: { fromWalletId: number; toWalletId: number; amount: number; fee: number }, sign: number) {
  return new Map([
    [t.fromWalletId, -(t.amount + t.fee) * sign],
    [t.toWalletId, t.amount * sign],
  ]);
}

export function TransferForm({ transferId, fromWalletId }: { transferId: number | null; fromWalletId: number | null }) {
  const { wallets, closeTx, changed } = useFinance();
  const toast = useToast();
  const confirm = useConfirm();
  const today = todayISO();

  const [original, setOriginal] = useState<Transfer | null>(null);
  const [loadError, setLoadError] = useState('');
  const [from, setFrom] = useState(fromWalletId ?? wallets[0]?.id ?? 0);
  const [to, setTo] = useState(() => wallets.find((w) => w.id !== (fromWalletId ?? wallets[0]?.id))?.id ?? 0);
  const [amount, setAmount] = useState(0);
  const [fee, setFee] = useState(0);
  const [note, setNote] = useState('');
  const [date, setDate] = useState(today);
  const [lastFee, setLastFee] = useState<number | null>(null);
  // True once the person picks a fee themselves; a prefill never overrides that.
  const [feeTouched, setFeeTouched] = useState(false);
  const chooseFee = (v: number) => {
    setFee(v);
    setFeeTouched(true);
  };
  const [errors, setErrors] = useState<Errors>({});
  const [saving, setSaving] = useState(false);

  // Editing: load the transfer behind the row that was clicked.
  useEffect(() => {
    if (!transferId) return;
    api.get<Transfer>(`/transfers/${transferId}`)
      .then((t) => {
        setOriginal(t);
        setFrom(t.fromWalletId);
        setTo(t.toWalletId);
        setAmount(t.amount);
        setFee(t.fee);
        setNote(t.note);
        setDate(t.occurredOn);
      })
      .catch((e: ApiError) => setLoadError(e.message));
  }, [transferId]);

  // Remember the fee per route. Prefill only when the person picks a route,
  // never over the values of the transfer being edited.
  useEffect(() => {
    setLastFee(null);
    if (!from || !to || from === to) return;
    let live = true;
    api.get<{ fee: number; found: boolean }>(`/transfers/last-fee${qs({ from, to })}`)
      .then((h) => {
        if (!live) return;
        const isOriginalRoute = original && original.fromWalletId === from && original.toWalletId === to;
        if (h.found) setLastFee(h.fee);
        if (isOriginalRoute || feeTouched) return;
        // A new route starts from its own history, not the previous route's fee.
        setFee(h.found ? h.fee : 0);
      })
      .catch(() => {});
    return () => { live = false; };
  // feeTouched is read, not watched: touching the fee must not refetch.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [from, to, original]);

  // An edited transfer may involve a wallet archived since; keep it selectable.
  const options: Wallet[] = useMemo(() => {
    if (!original) return wallets;
    const extra: Wallet[] = [];
    for (const [id, name] of [[original.fromWalletId, original.fromWalletName], [original.toWalletId, original.toWalletName]] as const) {
      if (!wallets.some((w) => w.id === id)) extra.push({ id, name, color: 'c0', balance: 0, type: 'other', archived: true });
    }
    return [...wallets, ...extra];
  }, [wallets, original]);

  const fromW = options.find((w) => w.id === from);
  const toW = options.find((w) => w.id === to);

  // Balances after saving, accounting for the old version when editing.
  const preview = useMemo(() => {
    const net = effects({ fromWalletId: from, toWalletId: to, amount, fee }, 1);
    if (original) {
      for (const [id, d] of effects(original, -1)) net.set(id, (net.get(id) ?? 0) + d);
    }
    return net;
  }, [from, to, amount, fee, original]);
  const afterFrom = fromW ? fromW.balance + (preview.get(from) ?? 0) : 0;
  const short = !!fromW && !fromW.archived && amount > 0 && afterFrom < 0;

  if (options.length < 2) {
    return (
      <Empty icon="transfer" title="Butuh minimal dua dompet"
        action={<Link to="/app/dompet" className="btn btn-primary" onClick={closeTx}>Tambah dompet</Link>}>
        Transfer memindahkan uang antar dompetmu sendiri, misalnya dari BCA ke GoPay.
      </Empty>
    );
  }
  if (loadError) return <div className="alert alert-danger" role="alert">{loadError}</div>;
  if (transferId && !original) {
    return <div style={{ display: 'grid', gap: 14 }}>{[56, 56, 70, 44].map((h, i) => <Skeleton key={i} h={h} />)}</div>;
  }

  function pickFrom(id: number) {
    setFrom(id);
    if (id === to) setTo(options.find((w) => w.id !== id)?.id ?? 0);
  }
  function swap() {
    setFrom(to);
    setTo(from);
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: Errors = {};
    if (!from) next.fromWalletId = 'Pilih dompet asal.';
    if (!to) next.toWalletId = 'Pilih dompet tujuan.';
    else if (from === to) next.toWalletId = 'Pilih dompet tujuan yang berbeda.';
    if (!amount) next.amount = 'Masukkan nominal yang dipindahkan.';
    setErrors(next);
    if (Object.keys(next).length) return;

    setSaving(true);
    const body = { fromWalletId: from, toWalletId: to, amount, fee, note, occurredOn: date };
    try {
      if (original) await api.put(`/transfers/${original.id}`, body);
      else await api.post('/transfers', body);
      toast(original ? 'Transfer diperbarui' : `${formatRp(amount)} dipindahkan ke ${toW?.name}`);
      changed();
      closeTx();
    } catch (err) {
      const e2 = err as ApiError;
      if (e2.code === 'INSUFFICIENT_BALANCE') setErrors({ amount: e2.message });
      else if (e2.field) setErrors({ [e2.field]: e2.message });
      else setErrors({ form: e2.message });
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    if (!original) return;
    const ok = await confirm({
      title: 'Hapus transfer?',
      body: (
        <>
          {formatRp(original.amount)} kembali ke <strong>{original.fromWalletName}</strong>
          {original.fee > 0 && <> beserta biaya admin {formatRp(original.fee)}</>}, dan saldo {original.toWalletName} dikurangi.
        </>
      ),
      confirmLabel: 'Hapus transfer',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/transfers/${original.id}`);
      toast('Transfer dihapus');
      changed();
      closeTx();
    } catch (err) {
      setErrors({ form: (err as ApiError).message });
    }
  }

  const walletChips = (selected: number, onPick: (id: number) => void, exclude: number, label: string) => (
    <div className="chips" role="group" aria-label={label}>
      {options.filter((w) => w.id !== exclude).map((w) => (
        <button key={w.id} type="button" className="chip" aria-pressed={selected === w.id} onClick={() => onPick(w.id)}>
          <span className={'dot swatch-' + w.color} />
          {w.name}
          {!w.archived && <small className="num">{formatRp(w.balance)}</small>}
        </button>
      ))}
    </div>
  );

  const isPreset = FEE_PRESETS.some((p) => p.value === fee);

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      <div className="route">
        <Field label="Dari" error={errors.fromWalletId}>{walletChips(from, pickFrom, 0, 'Dompet asal')}</Field>
        <button type="button" className="icon-btn route-swap" onClick={swap} disabled={!from || !to}
          aria-label="Tukar dompet asal dan tujuan" title="Tukar asal dan tujuan">
          <Icon name="swap" size={18} />
        </button>
        <Field label="Ke" error={errors.toWalletId}>{walletChips(to, setTo, from, 'Dompet tujuan')}</Field>
      </div>

      <Field label="Nominal yang dipindahkan" htmlFor="tf-amount" error={errors.amount}>
        <AmountInput id="tf-amount" value={amount} onChange={setAmount} invalid={!!errors.amount || short} autoFocus />
      </Field>

      <Field label="Biaya admin" htmlFor="tf-fee" error={errors.fee}
        hint={lastFee !== null
          ? `Transfer ${fromW?.name} → ${toW?.name} terakhir kena ${lastFee ? formatRp(lastFee) : 'gratis'}.`
          : 'Ditanggung dompet asal. Cek nominalnya di aplikasi bank atau e-wallet.'}>
        <div className="fee-row">
          <div className="chips" role="group" aria-label="Pilihan biaya admin">
            {FEE_PRESETS.map((p) => (
              <button key={p.value} type="button" className="chip" aria-pressed={fee === p.value} onClick={() => chooseFee(p.value)}>
                {p.label}
              </button>
            ))}
          </div>
          <label className={'fee-input' + (!isPreset ? ' custom' : '')}>
            <span>Rp</span>
            <input id="tf-fee" inputMode="numeric" autoComplete="off" value={groupDigits(fee)} placeholder="0"
              onChange={(e) => chooseFee(digitsOnly(e.target.value))} aria-invalid={!!errors.fee || undefined} />
          </label>
        </div>
      </Field>

      <Field label="Catatan (opsional)" htmlFor="tf-note" error={errors.note}>
        <input id="tf-note" className="input" value={note} maxLength={80} onChange={(e) => setNote(e.target.value)} placeholder="Contoh: Top up untuk ongkos minggu ini" />
      </Field>

      <Field label="Tanggal" htmlFor="tf-date" error={errors.occurredOn}>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <button type="button" className="chip" aria-pressed={date === today} onClick={() => setDate(today)}>Hari ini</button>
          <button type="button" className="chip" aria-pressed={date === shiftISO(today, -1)} onClick={() => setDate(shiftISO(today, -1))}>Kemarin</button>
          <input id="tf-date" type="date" className="input" style={{ width: 'auto', flex: 1, minWidth: 150 }} value={date} max={today} onChange={(e) => setDate(e.target.value)} />
        </div>
      </Field>

      {fromW && toW && from !== to && (
        <div className={'transfer-summary' + (short ? ' is-short' : '')} aria-live="polite">
          {[fromW, toW].map((w) => {
            const d = preview.get(w.id) ?? 0;
            return (
              <div key={w.id} className="ts-row">
                <span className="ts-wallet"><span className={'dot swatch-' + w.color} />{w.name}</span>
                {w.archived ? <span className="muted">diarsipkan</span> : (
                  <span className="num ts-change">
                    {formatRp(w.balance)} <Icon name="right" size={14} /> <strong>{formatRp(w.balance + d)}</strong>
                  </span>
                )}
                <span className={'num ts-delta ' + (d < 0 ? 'expense-text' : d > 0 ? 'income-text' : 'muted')}>{d === 0 ? '—' : formatRp(d, true)}</span>
              </div>
            );
          })}
          {fee > 0 && amount > 0 && (
            <p className="ts-note">Biaya {formatRp(fee)} dicatat sebagai pengeluaran kategori <strong>Biaya admin</strong>, jadi ikut terhitung di ringkasan.</p>
          )}
          {short && <p className="ts-note expense-text">Saldo {fromW.name} kurang {formatRp(-afterFrom)} untuk transfer ini.</p>}
        </div>
      )}

      {errors.form && <div className="alert alert-danger" role="alert">{errors.form}</div>}

      <div className="modal-actions">
        {original && (
          <button type="button" className="btn btn-quiet" style={{ marginRight: 'auto', color: 'var(--danger)' }} onClick={remove}>
            <Icon name="trash" size={18} /> Hapus
          </button>
        )}
        <button type="button" className="btn btn-ghost" onClick={closeTx}>Batal</button>
        <button type="submit" className="btn btn-primary" disabled={saving}>{saving ? 'Menyimpan…' : original ? 'Simpan' : 'Pindahkan'}</button>
      </div>
    </form>
  );
}
