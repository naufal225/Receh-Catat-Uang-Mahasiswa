// Shown at /app when no module provides a home page: the core alone.
import { brand } from '../brand';
import { Icon } from '../components/Icon';
import { Link } from '../components/ui';
import { useAuth } from '../lib/auth';

export default function Home() {
  const { user } = useAuth();
  return (
    <>
      <div className="page-head">
        <div>
          <h1>Halo, {user?.name}</h1>
          <p>Selamat datang di {brand.name}.</p>
        </div>
      </div>
      <div className="card card-pad" style={{ maxWidth: 620 }}>
        <p className="muted">Belum ada modul fitur yang dipasang. Akun, keamanan, notifikasi dan langganan sudah siap dipakai.</p>
        <div style={{ display: 'flex', gap: 10, marginTop: 16, flexWrap: 'wrap' }}>
          <Link to="/app/keamanan" className="btn btn-quiet"><Icon name="device" size={18} /> Perangkat &amp; keamanan</Link>
          <Link to="/app/notifikasi" className="btn btn-quiet"><Icon name="bell" size={18} /> Notifikasi</Link>
        </div>
      </div>
    </>
  );
}
