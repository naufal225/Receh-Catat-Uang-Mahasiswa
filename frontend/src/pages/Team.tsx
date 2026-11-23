// The active workspace: members and their roles, invitations, rename, leave,
// transfer, delete. A personal workspace has no members, so it only explains
// itself and offers to start a team.
import { useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { orgTitle, roleLabel } from '../components/OrgSwitcher';
import { Empty, ErrorNote, Field, Skeleton, useConfirm, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useApp } from '../lib/app';
import { useAuth } from '../lib/auth';
import { formatDateTime } from '../lib/format';
import { useFetch } from '../lib/hooks';
import { useOrg } from '../lib/org';
import { navigate } from '../lib/router';
import type { OrgInvite, OrgMembership, OrgPerson, OrgRole } from '../lib/types';

export default function Team() {
  const { active } = useOrg();
  if (!active) return <Skeleton h={240} />;
  return active.personal ? <Personal /> : <TeamView org={active} />;
}

function Personal() {
  const { active, switchTo, refresh } = useOrg();
  const { readOnly } = useApp();
  const toast = useToast();
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function create(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const o = await api.post<OrgMembership>('/orgs', { name });
      await refresh();
      switchTo(o.id);
      toast(`Tim “${o.name}” dibuat. Sekarang undang anggotanya.`);
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
          <h1>Tim</h1>
          <p>Kamu sedang di workspace {active ? orgTitle(active).toLowerCase() : 'pribadi'}: hanya kamu yang melihatnya.</p>
        </div>
      </div>
      <form className="card card-pad form-grid" style={{ maxWidth: 560 }} onSubmit={create} noValidate>
        <h2 style={{ fontSize: 18 }}>Buat tim</h2>
        <p className="muted">
          Tim punya dompet, anggaran, dan laporan sendiri, terpisah dari data pribadimu. Undang teman kos, panitia, atau keluarga, lalu atur siapa yang boleh apa.
        </p>
        <Field label="Nama tim" htmlFor="team-name" error={error}>
          <input id="team-name" className="input" value={name} maxLength={60} placeholder="Contoh: Kos Mawar"
            onChange={(e) => setName(e.target.value)} aria-invalid={!!error} disabled={readOnly} />
        </Field>
        <div><button type="submit" className="btn btn-primary" disabled={busy || readOnly || !name.trim()}>{busy ? 'Membuat…' : 'Buat tim'}</button></div>
      </form>
    </>
  );
}

function TeamView({ org }: { org: OrgMembership }) {
  const { user } = useAuth();
  const { readOnly, version, changed } = useApp();
  const { refresh } = useOrg();
  const confirm = useConfirm();
  const toast = useToast();
  const isOwner = org.role === 'owner';
  const canManage = org.role === 'owner' || org.role === 'admin';
  const members = useFetch<OrgPerson[]>(`/orgs/${org.id}/members?v=${version}`);
  const invites = useFetch<OrgInvite[]>(canManage ? `/orgs/${org.id}/invitations?v=${version}` : null);

  const reload = () => {
    members.reload();
    invites.reload();
  };

  async function run(action: () => Promise<unknown>, done: string) {
    try {
      await action();
      toast(done);
      reload();
      changed();
    } catch (err) {
      toast((err as ApiError).message);
    }
  }

  async function setRole(p: OrgPerson, role: OrgRole) {
    await run(() => api.put(`/orgs/${org.id}/members/${p.userId}`, { role }), `${p.name} sekarang ${roleLabel[role]}`);
  }

  async function remove(p: OrgPerson) {
    const self = p.userId === user?.id;
    const ok = await confirm({
      title: self ? 'Keluar dari tim?' : `Keluarkan ${p.name}?`,
      body: self ? `Kamu tidak akan bisa membuka data “${org.name}” lagi kecuali diundang kembali.` : `${p.name} tidak akan bisa membuka data “${org.name}” lagi.`,
      confirmLabel: self ? 'Keluar' : 'Keluarkan',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/orgs/${org.id}/members/${p.userId}`);
      if (self) {
        await refresh(); // falls back to the personal workspace
        navigate('/app');
        toast(`Kamu keluar dari “${org.name}”`);
      } else {
        toast(`${p.name} dikeluarkan`);
        reload();
        changed();
      }
    } catch (err) {
      toast((err as ApiError).message);
    }
  }

  async function transfer(p: OrgPerson) {
    const ok = await confirm({
      title: `Jadikan ${p.name} owner?`,
      body: 'Owner memegang langganan dan satu-satunya yang bisa menghapus tim. Kamu menjadi admin. Ini tidak bisa dibatalkan sendiri.',
      confirmLabel: 'Pindahkan kepemilikan',
      danger: true,
    });
    if (ok) await run(() => api.post(`/orgs/${org.id}/transfer`, { userId: p.userId }).then(refresh), `${p.name} sekarang owner`);
  }

  async function revoke(i: OrgInvite) {
    await run(() => api.del(`/orgs/${org.id}/invitations/${i.id}`), 'Undangan dibatalkan');
  }

  async function removeOrg() {
    const ok = await confirm({
      title: `Hapus “${org.name}”?`,
      body: 'Semua data tim (dompet, transaksi, anggaran, target) dihapus permanen untuk semua anggota. Data pribadimu tidak terpengaruh.',
      confirmLabel: 'Hapus tim',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/orgs/${org.id}`);
      await refresh();
      navigate('/app');
      toast('Tim dihapus');
    } catch (err) {
      toast((err as ApiError).message);
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{org.name}</h1>
          <p>Kamu {roleLabel[org.role].toLowerCase()} di tim ini. {canManage ? 'Kamu bisa mengatur anggota dan undangan.' : 'Hanya owner dan admin yang mengatur anggota.'}</p>
        </div>
      </div>

      <section className="card" style={{ overflow: 'hidden' }} aria-labelledby="team-members">
        <h2 id="team-members" style={{ fontSize: 16, padding: '16px 18px 0' }}>Anggota</h2>
        {members.error ? <ErrorNote message={members.error.message} onRetry={members.reload} /> : !members.data ? (
          <div style={{ padding: 18 }}><Skeleton h={120} /></div>
        ) : (
          <div className="table-scroll">
            <table className="table" style={{ width: '100%' }}>
              <thead><tr><th>Nama</th><th>Peran</th><th className="sr-only">Aksi</th></tr></thead>
              <tbody>
                {members.data.map((p) => {
                  const self = p.userId === user?.id;
                  const canRemove = !readOnly && p.role !== 'owner' && (self || isOwner || (org.role === 'admin' && p.role === 'member'));
                  return (
                    <tr key={p.userId}>
                      <td>
                        {p.name}{self && <span className="muted"> (kamu)</span>}
                        <span className="sub">{p.email}</span>
                      </td>
                      <td>
                        {isOwner && p.role !== 'owner' ? (
                          <select className="select" aria-label={`Peran ${p.name}`} value={p.role} disabled={readOnly}
                            onChange={(e) => setRole(p, e.target.value as OrgRole)}>
                            <option value="admin">Admin</option>
                            <option value="member">Anggota</option>
                          </select>
                        ) : (
                          <span className={'badge ' + (p.role === 'owner' ? 'badge-accent' : 'badge-neutral')}>{roleLabel[p.role]}</span>
                        )}
                      </td>
                      <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                        {isOwner && !self && p.role !== 'owner' && !readOnly && (
                          <button type="button" className="btn btn-quiet btn-sm" onClick={() => transfer(p)}>Jadikan owner</button>
                        )}
                        {canRemove && (
                          <button type="button" className="btn btn-quiet btn-sm" onClick={() => remove(p)}>{self ? 'Keluar' : 'Keluarkan'}</button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {canManage && (
        <>
          <InviteForm org={org} onSent={reload} />
          <section className="card card-pad" style={{ marginTop: 20 }} aria-labelledby="team-pending">
            <h2 id="team-pending" style={{ fontSize: 16 }}>Undangan yang belum diterima</h2>
            {!invites.data ? <Skeleton h={48} /> : invites.data.length === 0 ? (
              <Empty icon="users" title="Tidak ada undangan tertunda">Undangan berlaku 7 hari dan hanya bisa dipakai sekali.</Empty>
            ) : (
              <ul className="invite-list">
                {invites.data.map((i) => (
                  <li key={i.id}>
                    <span style={{ minWidth: 0 }}>
                      <strong>{i.email}</strong>
                      <small>{roleLabel[i.role]} · berlaku sampai {formatDateTime(i.expiresAt)}</small>
                    </span>
                    <button type="button" className="btn btn-quiet btn-sm" onClick={() => revoke(i)} disabled={readOnly}>Batalkan</button>
                  </li>
                ))}
              </ul>
            )}
          </section>
          <RenameForm org={org} onDone={refresh} />
        </>
      )}

      {isOwner && (
        <section className="card card-pad danger-zone" style={{ marginTop: 20 }} aria-labelledby="team-danger">
          <h2 id="team-danger" style={{ fontSize: 16 }}>Hapus tim</h2>
          <p className="muted">Menghapus tim menghapus semua datanya untuk semua anggota. Tidak bisa dibatalkan.</p>
          <div><button type="button" className="btn btn-danger" onClick={removeOrg} disabled={readOnly}>Hapus “{org.name}”</button></div>
        </section>
      )}
    </>
  );
}

function InviteForm({ org, onSent }: { org: OrgMembership; onSent: () => void }) {
  const { readOnly } = useApp();
  const toast = useToast();
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<OrgRole>('member');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [link, setLink] = useState<{ email: string; url: string } | null>(null);

  async function send(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const res = await api.post<{ invite: OrgInvite; token: string }>(`/orgs/${org.id}/invitations`, { email, role });
      setLink({ email: res.invite.email, url: `${window.location.origin}/undangan?t=${res.token}` });
      setEmail('');
      onSent();
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  async function copy() {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link.url);
      toast('Link disalin');
    } catch {
      toast('Salin manual: blok link di atas lalu tekan Ctrl+C');
    }
  }

  return (
    <form className="card card-pad form-grid" style={{ marginTop: 20 }} onSubmit={send} noValidate aria-labelledby="team-invite">
      <h2 id="team-invite" style={{ fontSize: 16 }}>Undang anggota</h2>
      <div className="form-row">
        <Field label="Email" htmlFor="inv-email" error={error}>
          <input id="inv-email" className="input" type="email" inputMode="email" autoComplete="off" value={email}
            placeholder="teman@email.com" onChange={(e) => setEmail(e.target.value)} aria-invalid={!!error} disabled={readOnly} />
        </Field>
        <Field label="Peran" htmlFor="inv-role">
          <select id="inv-role" className="select" value={role} onChange={(e) => setRole(e.target.value as OrgRole)} disabled={readOnly}>
            <option value="member">Anggota: memakai data tim</option>
            {org.role === 'owner' && <option value="admin">Admin: juga mengatur anggota</option>}
          </select>
        </Field>
      </div>
      <div><button type="submit" className="btn btn-primary" disabled={busy || readOnly || !email.trim()}>{busy ? 'Membuat…' : 'Buat undangan'}</button></div>
      {link && (
        <div className="invite-link" role="status">
          <p>Kirim link ini ke <strong>{link.email}</strong>. Link hanya tampil sekali dan hanya berlaku untuk email itu:</p>
          <div className="invite-link-row">
            <input className="input" readOnly value={link.url} aria-label="Link undangan" onFocus={(e) => e.currentTarget.select()} />
            <button type="button" className="btn btn-ghost" onClick={copy}><Icon name="copy" size={16} /> Salin</button>
          </div>
        </div>
      )}
    </form>
  );
}

function RenameForm({ org, onDone }: { org: OrgMembership; onDone: () => Promise<unknown> }) {
  const { readOnly } = useApp();
  const toast = useToast();
  const [name, setName] = useState(org.name);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.put(`/orgs/${org.id}`, { name });
      await onDone();
      toast('Nama tim disimpan');
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card card-pad form-grid" style={{ marginTop: 20, maxWidth: 560 }} onSubmit={save} noValidate aria-labelledby="team-rename">
      <h2 id="team-rename" style={{ fontSize: 16 }}>Nama tim</h2>
      <Field label="Nama" htmlFor="team-rename-input" error={error}>
        <input id="team-rename-input" className="input" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} aria-invalid={!!error} disabled={readOnly} />
      </Field>
      <div><button type="submit" className="btn btn-primary" disabled={busy || readOnly || !name.trim() || name.trim() === org.name}>{busy ? 'Menyimpan…' : 'Simpan'}</button></div>
    </form>
  );
}
