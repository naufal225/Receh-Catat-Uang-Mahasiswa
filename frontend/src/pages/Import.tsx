// Import a bank / e-wallet statement (CSV): pick the wallet and file, review
// every row (category, duplicates), then import as one batch that can be undone.
import { useMemo, useState } from 'react';
import { Icon } from '../components/Icon';
import { Empty, Field, Link, Skeleton, useConfirm, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useFinance } from '../lib/finance';
import { formatDate, formatDateTime, formatRp } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { ImportBatch, ImportPreview, ImportRow, StatementMapping } from '../lib/types';

const MAX_BYTES = 2.5 * 1024 * 1024;

export default function Import() {
  const { wallets, categories, membership, changed, version, readOnly } = useFinance();
  const toast = useToast();
  const confirm = useConfirm();
  const [picked, setWalletId] = useState(0);
  // Wallets load after the first render; until one is picked, use the first.
  const walletId = picked || wallets[0]?.id || 0;
  const [file, setFile] = useState<{ name: string; text: string } | null>(null);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [rows, setRows] = useState<(ImportRow & { include: boolean })[]>([]);
  const [mapping, setMapping] = useState<StatementMapping | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const { data: batches, reload } = useFetch<ImportBatch[]>(`/import/batches?v=${version}`);

  const allowed = membership?.limits.csvImport ?? false;

  async function runPreview(text: string, map: StatementMapping | null) {
    setBusy(true);
    setError('');
    try {
      const p = await api.post<ImportPreview>('/import/preview', { walletId, csv: text, mapping: map });
      setPreview(p);
      setMapping(p.mapping);
      setRows(p.rows.map((r) => ({ ...r, include: !r.duplicate })));
    } catch (e) {
      setError((e as ApiError).message);
      setPreview(null);
    } finally {
      setBusy(false);
    }
  }

  async function pickFile(f: File | undefined) {
    if (!f) return;
    if (f.size > MAX_BYTES) return setError('Berkas lebih dari 2,5 MB. Ekspor per bulan saja.');
    const text = await f.text();
    setFile({ name: f.name, text });
    runPreview(text, null);
  }

  const chosen = rows.filter((r) => r.include);
  const totals = useMemo(() => chosen.reduce((t, r) => (r.type === 'income' ? { ...t, in: t.in + r.amount } : { ...t, out: t.out + r.amount }), { in: 0, out: 0 }), [chosen]);

  async function commit() {
    setBusy(true);
    setError('');
    try {
      const b = await api.post<ImportBatch>('/import/commit', { walletId, fileName: file?.name ?? '', rows: chosen });
      toast(`${b.rows} transaksi diimpor ke ${b.walletName}`);
      setFile(null);
      setPreview(null);
      setRows([]);
      changed();
      reload();
    } catch (e) {
      setError((e as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  async function undo(b: ImportBatch) {
    const ok = await confirm({
      title: 'Batalkan impor?',
      body: <>{b.remaining} transaksi dari <strong>{b.fileName || 'impor ini'}</strong> dihapus dan saldo {b.walletName} dikembalikan.</>,
      confirmLabel: 'Batalkan impor', danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/import/batches/${b.id}`);
      toast('Impor dibatalkan');
      changed();
      reload();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  const setRow = (i: number, patch: Partial<ImportRow & { include: boolean }>) =>
    setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  const needsMapping = preview && preview.rows.length === 0 && mapping && (mapping.date < 0 || mapping.description < 0 || (mapping.amount < 0 && mapping.debit < 0 && mapping.credit < 0));

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Impor mutasi</h1>
          <p>Unggah mutasi CSV dari m-banking atau e-wallet, periksa, lalu impor sekaligus.</p>
        </div>
      </div>

      {!allowed ? (
        <div className="card card-pad">
          <Empty icon="upload" title="Impor mutasi adalah fitur Premium" action={<Link to="/app/langganan" className="btn btn-primary">Lihat Premium</Link>}>
            Catat puluhan transaksi sekaligus dari ekspor mutasi bank atau e-wallet, lengkap dengan deteksi duplikat.
          </Empty>
        </div>
      ) : (
        <div className="card card-pad form-grid" style={{ marginBottom: 20 }}>
          <Field label="Masukkan ke dompet">
            <div className="chips" role="group" aria-label="Dompet">
              {wallets.map((w) => (
                <button key={w.id} type="button" className="chip" aria-pressed={walletId === w.id}
                  onClick={() => { setWalletId(w.id); if (file) runPreview(file.text, mapping); }}>
                  <span className={'dot swatch-' + w.color} />{w.name}
                </button>
              ))}
            </div>
          </Field>
          <Field label="Berkas CSV" hint="Contoh: mutasi BCA (CSV), riwayat GoPay/DANA yang diekspor ke CSV. Maks. 1.000 baris.">
            <label className="file-drop">
              <Icon name="upload" size={20} />
              <span>{file ? file.name : 'Pilih berkas .csv'}</span>
              <input type="file" accept=".csv,text/csv" className="sr-only" disabled={readOnly || !walletId}
                onChange={(e) => { pickFile(e.target.files?.[0]); e.target.value = ''; }} />
            </label>
          </Field>
          {error && <div className="alert alert-danger" role="alert">{error}</div>}
          {busy && !preview && <Skeleton h={120} />}

          {needsMapping && preview && (
            <div className="alert alert-info" style={{ display: 'grid', gap: 12 }}>
              <span>Kolomnya belum dikenali. Tunjukkan kolom mana yang berisi tanggal, keterangan, dan nominal.</span>
              <div className="form-row">
                {(['date', 'description', 'amount'] as const).map((k) => (
                  <Field key={k} label={{ date: 'Tanggal', description: 'Keterangan', amount: 'Nominal' }[k]}>
                    <select className="select" value={mapping![k]} onChange={(e) => setMapping({ ...mapping!, [k]: Number(e.target.value) })}>
                      <option value={-1}>—</option>
                      {(preview.headers ?? []).map((h, i) => <option key={i} value={i}>{h || `Kolom ${i + 1}`}</option>)}
                    </select>
                  </Field>
                ))}
              </div>
              <div><button className="btn btn-primary btn-sm" onClick={() => runPreview(file!.text, { ...mapping!, headerRow: Math.max(0, mapping!.headerRow) })}>Baca ulang</button></div>
            </div>
          )}

          {preview && rows.length > 0 && (
            <>
              <div className="import-summary">
                <strong>{chosen.length} dari {rows.length} baris dipilih</strong>
                <span className="income-text num">+ {formatRp(totals.in)}</span>
                <span className="expense-text num">− {formatRp(totals.out)}</span>
                {preview.skipped > 0 && <span className="muted">{preview.skipped} baris bukan transaksi dilewati</span>}
                {rows.some((r) => r.duplicate) && <span className="muted">Baris yang sudah tercatat tidak dicentang</span>}
              </div>
              <div className="table-scroll import-table">
                <table className="table">
                  <thead><tr><th><span className="sr-only">Pilih</span></th><th>Tanggal</th><th>Keterangan</th><th>Kategori</th><th style={{ textAlign: 'right' }}>Nominal</th></tr></thead>
                  <tbody>
                    {rows.map((r, i) => (
                      <tr key={r.line} style={r.include ? undefined : { opacity: 0.5 }}>
                        <td><input type="checkbox" checked={r.include} onChange={(e) => setRow(i, { include: e.target.checked })} aria-label={`Impor ${r.name}`} /></td>
                        <td className="num" style={{ whiteSpace: 'nowrap' }}>{formatDate(r.date, false)}</td>
                        <td>{r.name}{r.duplicate && <span className="badge badge-warn" style={{ marginLeft: 6 }}>Sudah ada</span>}</td>
                        <td>
                          <select className="select select-sm" value={r.categoryId} onChange={(e) => setRow(i, { categoryId: Number(e.target.value) })} aria-label={`Kategori ${r.name}`}>
                            {categories.filter((c) => c.type === r.type).map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                          </select>
                        </td>
                        <td className={'num ' + (r.type === 'income' ? 'income-text' : 'expense-text')} style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                          {formatRp(r.type === 'income' ? r.amount : -r.amount, true)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div>
                <button className="btn btn-primary" onClick={commit} disabled={busy || readOnly || chosen.length === 0}>
                  {busy ? 'Mengimpor…' : `Impor ${chosen.length} transaksi`}
                </button>
              </div>
            </>
          )}
          {preview && rows.length === 0 && !needsMapping && <Empty icon="list" title="Tidak ada transaksi di berkas ini" />}
        </div>
      )}

      {batches && batches.length > 0 && (
        <div className="card" style={{ overflow: 'hidden' }}>
          <div className="card-pad" style={{ paddingBottom: 0 }}><h2 style={{ fontSize: 17, fontFamily: 'var(--font-sans)', fontWeight: 750 }}>Riwayat impor</h2></div>
          <div className="ent-list" style={{ border: 0 }}>
            {batches.map((b) => (
              <div key={b.id} className="ent-row" style={{ padding: '12px 22px' }}>
                <div>
                  <strong>{b.fileName || 'Impor'} → {b.walletName}</strong>
                  <span className="sub">{formatDateTime(b.createdAt)} · {b.remaining} transaksi · +{formatRp(b.totalIn)} / −{formatRp(b.totalOut)}</span>
                </div>
                {!readOnly && b.remaining > 0 && <button className="btn btn-ghost btn-sm" onClick={() => undo(b)}>Batalkan</button>}
              </div>
            ))}
          </div>
        </div>
      )}
    </>
  );
}
