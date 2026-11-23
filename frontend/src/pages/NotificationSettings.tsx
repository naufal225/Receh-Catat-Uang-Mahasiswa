// Which reminders to receive and the settings modules expose (a threshold,
// an hour). Everything listed here is declared by modules on the server.
import { useEffect, useState, type FormEvent } from 'react';
import { pick } from '../brand';
import { Icon, iconOr } from '../components/Icon';
import { AmountInput, Empty, Field, Skeleton, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useApp } from '../lib/app';
import type { NotificationPrefs } from '../lib/types';

type Setting = NotificationPrefs['settings'][number];

function SettingInput({ s, value, onChange, disabled }: { s: Setting; value: number; onChange: (v: number) => void; disabled: boolean }) {
  const id = 'np-' + s.key.replace(/\W/g, '-');
  if (s.kind === 'amount') return <AmountInput id={id} value={value} onChange={onChange} />;
  if (s.kind === 'hour') {
    return (
      <select id={id} className="select" value={value} disabled={disabled} onChange={(e) => onChange(Number(e.target.value))}>
        {Array.from({ length: 24 }, (_, h) => <option key={h} value={h}>{String(h).padStart(2, '0')}.00</option>)}
      </select>
    );
  }
  return <input id={id} className="input" type="number" inputMode="numeric" min={s.min} max={s.max} value={value} disabled={disabled}
    onChange={(e) => onChange(Number(e.target.value))} />;
}

export default function NotificationSettings() {
  const toast = useToast();
  const { readOnly } = useApp();
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
      const body = {
        kinds: Object.fromEntries(prefs.kinds.map((k) => [k.key, k.enabled])),
        settings: Object.fromEntries(prefs.settings.map((s) => [s.key, s.value])),
      };
      setPrefs(await api.put<NotificationPrefs>('/notifications/prefs', body));
      toast('Pengaturan notifikasi disimpan');
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  const setKind = (key: string, enabled: boolean) =>
    prefs && setPrefs({ ...prefs, kinds: prefs.kinds.map((k) => (k.key === key ? { ...k, enabled } : k)) });
  const setSetting = (key: string, value: number) =>
    prefs && setPrefs({ ...prefs, settings: prefs.settings.map((s) => (s.key === key ? { ...s, value } : s)) });

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
      ) : prefs.kinds.length === 0 ? (
        <div className="card"><Empty icon="bell" title="Belum ada jenis notifikasi">Modul yang dipasang belum mengirim pengingat apa pun.</Empty></div>
      ) : (
        <form className="card card-pad form-grid" style={{ maxWidth: 620 }} onSubmit={save} noValidate>
          <fieldset className="toggle-list" disabled={readOnly}>
            <legend className="sr-only">Jenis notifikasi</legend>
            {prefs.kinds.map((k) => (
              <label key={k.key} className="toggle-row">
                <span className="inbox-icon tone-neutral"><Icon name={iconOr(k.icon)} size={16} /></span>
                <span style={{ flex: 1 }}>{pick(k.label)}</span>
                <input type="checkbox" className="switch" checked={k.enabled} onChange={(e) => setKind(k.key, e.target.checked)} />
              </label>
            ))}
          </fieldset>
          {prefs.settings.length > 0 && (
            <div className="form-row">
              {prefs.settings.map((s) => (
                <Field key={s.key} label={pick(s.label)} htmlFor={'np-' + s.key.replace(/\W/g, '-')}>
                  <SettingInput s={s} value={s.value} disabled={readOnly} onChange={(v) => setSetting(s.key, v)} />
                </Field>
              ))}
            </div>
          )}
          {error && <div className="alert alert-danger">{error}</div>}
          <div><button type="submit" className="btn btn-primary" disabled={busy || readOnly}>{busy ? 'Menyimpan…' : 'Simpan'}</button></div>
        </form>
      )}
    </>
  );
}
