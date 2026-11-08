import { useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { ColorPicker, Field, Modal, Segmented, useConfirm, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useFinance } from '../lib/finance';
import type { Category, TxType } from '../lib/types';

export default function Categories() {
  const { categories, changed, readOnly } = useFinance();
  const toast = useToast();
  const confirm = useConfirm();
  const [editing, setEditing] = useState<Category | { type: TxType } | null>(null);

  async function remove(c: Category) {
    const ok = await confirm({ title: `Hapus kategori ${c.name}?`, body: 'Kategori yang masih dipakai transaksi atau jadwal rutin tidak bisa dihapus.', confirmLabel: 'Hapus', danger: true });
    if (!ok) return;
    try {
      await api.del(`/categories/${c.id}`);
      toast('Kategori dihapus');
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  const section = (type: TxType, title: string) => (
    <div className="card" style={{ overflow: 'hidden' }}>
      <div className="list-item" style={{ background: 'var(--surface-2)' }}>
        <div className="grow"><strong>{title}</strong></div>
        {!readOnly && <button className="btn btn-ghost btn-sm" onClick={() => setEditing({ type })}><Icon name="plus" size={16} /> Tambah</button>}
      </div>
      <div className="list-card">
        {categories.filter((c) => c.type === type).map((c) => (
          <div key={c.id} className="list-item">
            <span className={'dot swatch-' + c.color} style={{ width: 14, height: 14 }} />
            <div className="grow">{c.name}</div>
            {!readOnly && (
              <div className="row-actions">
                <button className="icon-btn" onClick={() => setEditing(c)} aria-label={`Ubah ${c.name}`}><Icon name="edit" size={18} /></button>
                <button className="icon-btn" onClick={() => remove(c)} aria-label={`Hapus ${c.name}`}><Icon name="trash" size={18} /></button>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Kategori</h1>
          <p>Kelompokkan transaksi supaya kelihatan uang paling banyak lari ke mana.</p>
        </div>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 20 }}>
        {section('expense', 'Pengeluaran')}
        {section('income', 'Pemasukan')}
      </div>
      <Modal open={editing !== null} onClose={() => setEditing(null)} title={editing && 'id' in editing ? 'Ubah kategori' : 'Tambah kategori'}>
        {editing && <CategoryForm key={'id' in editing ? editing.id : 'new-' + editing.type} initial={editing} onDone={() => { setEditing(null); changed(); }} />}
      </Modal>
    </>
  );
}

function CategoryForm({ initial, onDone }: { initial: Category | { type: TxType }; onDone: () => void }) {
  const existing = 'id' in initial ? initial : null;
  const toast = useToast();
  const [name, setName] = useState(existing?.name ?? '');
  const [type, setType] = useState<TxType>(initial.type);
  const [color, setColor] = useState(existing?.color ?? 'c1');
  const [error, setError] = useState('');

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!name.trim()) return setError('Nama kategori wajib diisi.');
    try {
      if (existing) await api.put(`/categories/${existing.id}`, { name, color, type });
      else await api.post('/categories', { name, type, color });
      toast(existing ? 'Kategori diperbarui' : `Kategori ${name} ditambahkan`);
      onDone();
    } catch (err) {
      setError((err as ApiError).message);
    }
  }

  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      {!existing && (
        <Segmented label="Jenis kategori" value={type} onChange={setType} big
          options={[{ value: 'expense', label: 'Pengeluaran', tone: 'expense' }, { value: 'income', label: 'Pemasukan', tone: 'income' }]} />
      )}
      <Field label="Nama kategori" htmlFor="c-name" error={error}>
        <input id="c-name" className="input" value={name} maxLength={30} onChange={(e) => setName(e.target.value)} placeholder="Contoh: Laundry" autoFocus />
      </Field>
      <Field label="Warna">
        <ColorPicker value={color} onChange={setColor} />
      </Field>
      <div className="modal-actions">
        <button type="submit" className="btn btn-primary">Simpan</button>
      </div>
    </form>
  );
}
