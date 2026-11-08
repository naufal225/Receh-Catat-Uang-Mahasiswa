// The global "Catat" modal: one place to record an expense, an income or a
// transfer between wallets. Designed to finish in a few taps: kind, amount,
// name, then wallet and category as chips.
import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { api, ApiError } from '../lib/api';
import { useFinance, type EntryMode } from '../lib/finance';
import { formatRp, shiftISO, todayISO } from '../lib/format';
import type { TxType } from '../lib/types';
import { TransferForm } from './TransferForm';
import { AmountInput, Empty, Field, Link, Modal, Segmented, useToast } from './ui';

type Errors = Partial<Record<'amount' | 'name' | 'walletId' | 'categoryId' | 'occurredOn' | 'form', string>>;

export function TransactionModal() {
  const { txModal, closeTx } = useFinance();
  const title = txModal.transferId ? 'Ubah transfer' : txModal.edit ? 'Ubah transaksi' : 'Catat transaksi';
  // A fresh body per opening, so no state leaks from the previous entry.
  const key = [txModal.open, txModal.edit?.id, txModal.transferId, txModal.mode, txModal.fromWalletId].join(':');
  return (
    <Modal open={txModal.open} onClose={closeTx} title={title}>
      <EntryBody key={key} />
    </Modal>
  );
}

function EntryBody() {
  const { txModal } = useFinance();
  const [mode, setMode] = useState<EntryMode>(txModal.mode);
  const editing = !!txModal.edit || !!txModal.transferId;

  // A new entry may switch between all three kinds. An edited transaction may
  // flip between income and expense; a transfer stays a transfer.
  const options = [
    { value: 'expense' as const, label: 'Pengeluaran', tone: 'expense' },
    { value: 'income' as const, label: 'Pemasukan', tone: 'income' },
    ...(editing ? [] : [{ value: 'transfer' as const, label: 'Transfer', tone: 'transfer' }]),
  ];

  return (
    <div className="form-grid">
      {!txModal.transferId && <Segmented big label="Jenis transaksi" value={mode} onChange={setMode} options={options} />}
      {mode === 'transfer'
        ? <TransferForm transferId={txModal.transferId} fromWalletId={txModal.fromWalletId} />
        : <TransactionForm type={mode} />}
    </div>
  );
}

function TransactionForm({ type }: { type: TxType }) {
  const { wallets, categories, txModal, closeTx, changed } = useFinance();
  const toast = useToast();
  const edit = txModal.edit;
  const today = todayISO();

  const [amount, setAmount] = useState(edit?.amount ?? 0);
  const [name, setName] = useState(edit?.name ?? '');
  const [walletId, setWalletId] = useState(edit?.walletId ?? txModal.fromWalletId ?? wallets[0]?.id ?? 0);
  const [categoryId, setCategoryId] = useState(edit?.categoryId ?? 0);
  const [date, setDate] = useState(edit?.occurredOn ?? today);
  const [errors, setErrors] = useState<Errors>({});
  const [saving, setSaving] = useState(false);

  const cats = useMemo(() => categories.filter((c) => c.type === type), [categories, type]);
  // When the type flips, a category of the other type no longer applies.
  useEffect(() => {
    if (!cats.some((c) => c.id === categoryId)) setCategoryId(0);
  }, [cats, categoryId]);

  // An edited transaction may sit in an archived wallet that is no longer listed.
  const walletOptions = edit && !wallets.some((w) => w.id === edit.walletId)
    ? [...wallets, { id: edit.walletId, name: edit.walletName, color: edit.walletColor, balance: 0, type: 'other' as const, archived: true }]
    : wallets;
  const selectedWallet = walletOptions.find((w) => w.id === walletId);

  if (wallets.length === 0 && !edit) {
    return (
      <Empty icon="wallet" title="Tambahkan dompet dulu" action={<Link to="/app/dompet" className="btn btn-primary" onClick={closeTx}>Tambah dompet</Link>}>
        Setiap transaksi masuk ke dompet, misalnya BCA, GoPay atau uang tunai.
      </Empty>
    );
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: Errors = {};
    if (!amount) next.amount = 'Masukkan nominalnya.';
    if (!name.trim()) next.name = 'Beri nama kegiatannya.';
    if (!walletId) next.walletId = 'Pilih dompet.';
    if (!categoryId) next.categoryId = 'Pilih kategori.';
    setErrors(next);
    if (Object.keys(next).length) return;

    setSaving(true);
    const body = { type, amount, name, walletId, categoryId, occurredOn: date };
    try {
      if (edit) await api.put(`/transactions/${edit.id}`, body);
      else await api.post('/transactions', body);
      toast(edit ? 'Transaksi diperbarui' : `${type === 'expense' ? 'Pengeluaran' : 'Pemasukan'} ${formatRp(amount)} tercatat`);
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

  const remaining = selectedWallet && type === 'expense' && amount > 0 && !edit ? selectedWallet.balance - amount : null;

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      <Field label="Nominal" htmlFor="tx-amount" error={errors.amount}
        hint={remaining !== null ? (remaining >= 0 ? `Sisa saldo ${selectedWallet!.name}: ${formatRp(remaining)}` : `Saldo ${selectedWallet!.name} hanya ${formatRp(selectedWallet!.balance)}`) : undefined}>
        <AmountInput id="tx-amount" value={amount} onChange={setAmount} invalid={!!errors.amount || (remaining !== null && remaining < 0)} autoFocus />
      </Field>

      <Field label="Nama kegiatan" htmlFor="tx-name" error={errors.name}>
        <input id="tx-name" className="input" value={name} maxLength={80} onChange={(e) => setName(e.target.value)}
          placeholder={type === 'expense' ? 'Contoh: Makan siang di kantin' : 'Contoh: Kiriman orang tua'} aria-invalid={!!errors.name || undefined} />
      </Field>

      <Field label="Dompet" error={errors.walletId}>
        <div className="chips" role="group" aria-label="Dompet">
          {walletOptions.map((w) => (
            <button key={w.id} type="button" className="chip" aria-pressed={walletId === w.id} onClick={() => setWalletId(w.id)}>
              <span className={'dot swatch-' + w.color} />
              {w.name}
              {!w.archived && <small className="num">{formatRp(w.balance)}</small>}
            </button>
          ))}
        </div>
      </Field>

      <Field label="Kategori" error={errors.categoryId}>
        <div className="chips" role="group" aria-label="Kategori">
          {cats.map((c) => (
            <button key={c.id} type="button" className="chip" aria-pressed={categoryId === c.id} onClick={() => setCategoryId(c.id)}>
              <span className={'dot swatch-' + c.color} />
              {c.name}
            </button>
          ))}
        </div>
      </Field>

      <Field label="Tanggal" htmlFor="tx-date" error={errors.occurredOn}>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <button type="button" className="chip" aria-pressed={date === today} onClick={() => setDate(today)}>Hari ini</button>
          <button type="button" className="chip" aria-pressed={date === shiftISO(today, -1)} onClick={() => setDate(shiftISO(today, -1))}>Kemarin</button>
          <input id="tx-date" type="date" className="input" style={{ width: 'auto', flex: 1, minWidth: 150 }} value={date} max={today} onChange={(e) => setDate(e.target.value)} />
        </div>
      </Field>

      {errors.form && <div className="alert alert-danger" role="alert">{errors.form}</div>}

      <div className="modal-actions">
        <button type="button" className="btn btn-ghost" onClick={closeTx}>Batal</button>
        <button type="submit" className="btn btn-primary" disabled={saving}>{saving ? 'Menyimpan…' : 'Simpan'}</button>
      </div>
    </form>
  );
}
