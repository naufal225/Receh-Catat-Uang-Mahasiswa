// Shows the active workspace and lets the user switch or start a team. "side"
// is the full-width button in the sidebar; "chip" is the compact one in the
// mobile header.
import { useState, type FormEvent } from 'react';
import { api, ApiError } from '../lib/api';
import { useApp } from '../lib/app';
import { useOrg } from '../lib/org';
import { navigate } from '../lib/router';
import type { OrgMembership, OrgRole } from '../lib/types';
import { Icon } from './Icon';
import { Field, Modal, useToast } from './ui';

export const roleLabel: Record<OrgRole, string> = { owner: 'Owner', admin: 'Admin', member: 'Anggota' };

export const orgTitle = (o: Pick<OrgMembership, 'name' | 'personal'>) => (o.personal ? 'Pribadi' : o.name);

export function OrgSwitcher({ variant = 'side' }: { variant?: 'side' | 'chip' }) {
  const { orgs, active, switchTo, refresh } = useOrg();
  const { readOnly } = useApp();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  if (!active) return null;

  function pick(id: number) {
    switchTo(id);
    setOpen(false);
  }

  async function create(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const o = await api.post<OrgMembership>('/orgs', { name });
      await refresh();
      switchTo(o.id);
      setName('');
      setOpen(false);
      toast(`Workspace “${o.name}” dibuat`);
      navigate('/app/tim');
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <button type="button" className={variant === 'chip' ? 'org-chip' : 'org-switch'} onClick={() => setOpen(true)}
        aria-haspopup="dialog" aria-label={`Workspace aktif: ${orgTitle(active)}. Ganti workspace`}>
        <Icon name={active.personal ? 'home' : 'users'} size={variant === 'chip' ? 14 : 18} />
        <span className="org-switch-text">
          <strong>{orgTitle(active)}</strong>
          {variant === 'side' && <small>{active.personal ? 'Workspace pribadi' : roleLabel[active.role]}</small>}
        </span>
        {variant === 'side' && <Icon name="chevron" size={16} />}
      </button>
      <Modal open={open} onClose={() => setOpen(false)} title="Pindah workspace">
        <ul className="org-list">
          {orgs.map((o) => (
            <li key={o.id}>
              <button type="button" className="org-row" aria-current={o.id === active.id ? 'true' : undefined} onClick={() => pick(o.id)}>
                <span className="inbox-icon tone-neutral"><Icon name={o.personal ? 'home' : 'users'} size={16} /></span>
                <span style={{ flex: 1, minWidth: 0 }}>
                  <strong>{orgTitle(o)}</strong>
                  <small>{o.personal ? 'Hanya kamu' : roleLabel[o.role]}</small>
                </span>
                {o.id === active.id && <Icon name="check" size={18} />}
              </button>
            </li>
          ))}
        </ul>
        <form className="org-create" onSubmit={create} noValidate>
          <Field label="Buat tim baru" htmlFor="org-new" error={error} hint="Dompet, anggaran, dan laporan tim terpisah dari data pribadimu.">
            <input id="org-new" className="input" value={name} maxLength={60} placeholder="Contoh: Kos Mawar"
              onChange={(e) => setName(e.target.value)} aria-invalid={!!error} disabled={readOnly} />
          </Field>
          <button type="submit" className="btn btn-primary" disabled={busy || readOnly || !name.trim()}>
            {busy ? 'Membuat…' : 'Buat tim'}
          </button>
        </form>
      </Modal>
    </>
  );
}
