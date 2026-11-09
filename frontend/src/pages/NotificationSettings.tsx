// Which reminders to receive, the low-balance threshold, and the hour of the
// evening reminder.
import { useEffect, useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { kindMeta } from '../components/Notifications';
import { AmountInput, Field, Skeleton, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useFinance } from '../lib/finance';
import type { NotificationKind, NotificationPrefs } from '../lib/types';

const order: NotificationKind[] = ['budget_warning', 'budget_over', 'bill_due', 'bill_failed', 'low_balance', 'goal_achieved', 'daily_reminder', 'weekly_summary'];

export default function NotificationSettings() {
  const toast = useToast();
  const { readOnly } = useFinance();
  const [prefs, setPrefs] = useState<NotificationPrefs | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.get<NotificationPrefs>('/notifications/prefs').then(setPrefs).catch((e: ApiError) => setError(e.message));
  }, []);

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!prefs) return;
    setBusy(true);
    setError('');
    try {
      setPrefs(await api.put<NotificationPrefs>('/notifications/prefs', prefs));
      toast('Pengaturan notifikasi disimpan');
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Notifikasi</h1>
          <p>Pilih pengingat yang kamu mau. Notifikasi tampil di ikon lonceng.</p>
        </div>
      </div>
      {!prefs ? (
        error ? <div className="alert alert-danger">{error}</div> : <Skeleton h={320} />
      ) : (
        <form className="card card-pad form-grid" style={{ maxWidth: 620 }} onSubmit={save} noValidate>
          <fieldset className="toggle-list" disabled={readOnly}>
            <legend className="sr-only">Jenis notifikasi</legend>
            {order.map((k) => (
              <label key={k} className="toggle-row">
                <span className="inbox-icon tone-neutral"><Icon name={kindMeta[k].icon} size={16} /></span>
                <span style={{ flex: 1 }}>{kindMeta[k].label}</span>
                <input type="checkbox" className="switch" checked={prefs.kinds[k]}
                  onChange={(e) => setPrefs({ ...prefs, kinds: { ...prefs.kinds, [k]: e.target.checked } })} />
              </label>
            ))}
          </fieldset>
          <div className="form-row">
            <Field label="Saldo dianggap menipis di bawah" htmlFor="np-low" hint="Berlaku untuk semua dompet yang sudah dipakai.">
              <AmountInput id="np-low" value={prefs.lowBalanceThreshold} onChange={(v) => setPrefs({ ...prefs, lowBalanceThreshold: v })} />
            </Field>
            <Field label="Pengingat malam mulai pukul" htmlFor="np-hour">
              <select id="np-hour" className="select" value={prefs.reminderHour} disabled={readOnly}
                onChange={(e) => setPrefs({ ...prefs, reminderHour: Number(e.target.value) })}>
                {Array.from({ length: 24 }, (_, h) => <option key={h} value={h}>{String(h).padStart(2, '0')}.00</option>)}
              </select>
            </Field>
          </div>
          {error && <div className="alert alert-danger">{error}</div>}
          <div><button type="submit" className="btn btn-primary" disabled={busy || readOnly}>{busy ? 'Menyimpan…' : 'Simpan'}</button></div>
        </form>
      )}
    </>
  );
}
