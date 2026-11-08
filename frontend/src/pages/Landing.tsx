import { Icon, Logo, type IconName } from '../components/Icon';
import { Link } from '../components/ui';
import { useAuth } from '../lib/auth';

const features: { icon: IconName; title: string; body: string }[] = [
  { icon: 'wallet', title: 'Per dompet, seperti aslinya', body: 'BCA, GoPay, DANA, uang tunai: setiap transaksi masuk ke dompetnya, dan saldo langsung menyesuaikan.' },
  { icon: 'tag', title: 'Kategori yang masuk akal', body: 'Makan, minum, listrik, kuliah, langganan. Sudah disiapkan, tinggal pakai, dan bisa ditambah sendiri.' },
  { icon: 'repeat', title: 'Tagihan rutin tercatat sendiri', body: 'Netflix Rp 54 rb tiap bulan, listrik kos, uang saku mingguan: atur sekali, tercatat otomatis saat jatuh tempo.' },
  { icon: 'chart', title: 'Grafik yang bisa diajak ngobrol', body: 'Lihat per hari, minggu, bulan, atau tahun. Klik batang grafik untuk melihat ke mana uang periode itu pergi.' },
  { icon: 'file', title: 'Laporan PDF bulanan', body: 'Satu klik untuk laporan rapi: ringkasan, per dompet, per kategori, dan semua transaksi.' },
  { icon: 'device', title: 'Pantau perangkat yang masuk', body: 'Lihat perangkat, lokasi, dan IP yang sedang memakai akunmu, lalu keluarkan yang tidak dikenal.' },
];

export default function Landing() {
  const { user } = useAuth();
  const home = user ? (user.role === 'admin' ? '/admin' : '/app') : null;
  return (
    <div className="landing">
      <header className="l-nav">
        <Link to="/" className="brand"><Logo /> Receh</Link>
        <nav aria-label="Navigasi">
          <a href="#fitur" className="btn btn-quiet hide-sm">Fitur</a>
          <a href="#cara" className="btn btn-quiet hide-sm">Cara pakai</a>
          {home ? (
            <Link to={home} className="btn btn-primary">Buka aplikasi</Link>
          ) : (
            <>
              <Link to="/masuk" className="btn btn-ghost">Masuk</Link>
              <Link to="/daftar" className="btn btn-primary">Daftar</Link>
            </>
          )}
        </nav>
      </header>

      <main>
        <section className="l-hero">
          <div>
            <span className="l-kicker reveal"><Icon name="sparkle" size={16} /> Dibuat untuk dompet mahasiswa</span>
            <h1 className="reveal reveal-2">Uang saku <span className="hl">nggak lagi</span> hilang entah ke mana.</h1>
            <p className="reveal reveal-3">Receh mencatat dan merencanakan pemasukan dan pengeluaranmu per dompet, dari rekening BCA sampai GoPay, dalam tampilan yang langsung dipahami sekali lihat.</p>
            <div className="l-cta reveal reveal-4">
              <Link to={home ?? '/daftar'} className="btn btn-primary">{home ? 'Buka aplikasi' : 'Daftar gratis'} <Icon name="right" size={18} /></Link>
              <a href="#fitur" className="btn btn-ghost">Lihat fitur</a>
            </div>
            {/* <div className="l-proof reveal reveal-4">
              <span><Icon name="check" size={16} /> Tanpa OTP atau verifikasi email</span>
              <span><Icon name="check" size={16} /> Gratis</span>
            </div> */}
          </div>
          <div className="l-visual" aria-hidden="true">
            <div className="wallet-card wallet-bg-c1 float-a">
              <div className="wc-type">Rekening bank</div>
              <div className="wc-name">BCA</div>
              <div className="wc-balance">Rp 1.482.000</div>
            </div>
            <div className="receipt">
              <div className="receipt-head">
                <div className="mono">Struk terbaru</div>
                <strong style={{ fontSize: 18 }}>Minggu ini</strong>
              </div>
              <div className="receipt-list">
                {[
                  ['Kiriman bulanan', 'Uang Saku · BCA', '+ Rp 1.500.000', true, 'c3'],
                  ['Nasi padang', 'Makan · GoPay', '− Rp 18.000', false, 'c1'],
                  ['Es teh', 'Minum · Tunai', '− Rp 5.000', false, 'c2'],
                  ['Token listrik', 'Listrik · DANA', '− Rp 50.000', false, 'c4'],
                ].map(([n, m, a, inc, c]) => (
                  <div className="tx-row" key={n as string}>
                    <div className={'tx-icon swatch-' + c}>{(n as string)[0]}</div>
                    <div className="tx-main"><div className="tx-name">{n}</div><div className="tx-meta">{m}</div></div>
                    <div className={'tx-amount ' + (inc ? 'income-text' : 'expense-text')}>{a}</div>
                  </div>
                ))}
              </div>
              <div className="receipt-foot"><span>Sisa</span><strong>Rp 1.427.000</strong></div>
            </div>
            <div className="wallet-card wallet-bg-c3 float-b">
              <div className="wc-type">E-wallet</div>
              <div className="wc-name">GoPay</div>
              <div className="wc-balance">Rp 212.500</div>
            </div>
          </div>
        </section>

        <section className="l-section" id="fitur">
          <h2>Semua yang kamu butuhkan, tanpa ribet.</h2>
          <p>Tidak perlu jago akuntansi. Kalau bisa pakai e-wallet, kamu bisa pakai Receh.</p>
          <div className="features">
            {features.map((f) => (
              <div key={f.title} className="card feature">
                <div className="f-icon"><Icon name={f.icon} size={22} /></div>
                <h3>{f.title}</h3>
                <p>{f.body}</p>
              </div>
            ))}
          </div>
        </section>

        <section className="l-section" id="cara">
          <h2>Mulai dalam tiga langkah.</h2>
          <p>Kurang dari dua menit dari daftar sampai transaksi pertama tercatat.</p>
          <div className="steps">
            <div className="card step"><h3>Daftar</h3><p>Cukup nama, email, dan kata sandi. Tidak ada OTP.</p></div>
            <div className="card step"><h3>Tambah dompet</h3><p>Pilih BCA, GoPay, DANA, atau tunai, lalu isi saldonya sekarang.</p></div>
            <div className="card step"><h3>Catat & pantau</h3><p>Tekan “Catat”, isi nominal, selesai. Grafik dan saldo langsung diperbarui.</p></div>
          </div>
        </section>

        <div className="l-band">
          <section className="l-section">
            <h2>Akhir bulan nggak perlu <em>deg-degan</em> lagi.</h2>
            <p>Lihat sisa uangmu setelah semua tagihan rutin, sebelum tanggal tua datang.</p>
            <Link to={home ?? '/daftar'} className="btn btn-accent" style={{ minHeight: 50, padding: '0 26px', fontSize: 16 }}>
              {home ? 'Buka aplikasi' : 'Buat akun gratis'} <Icon name="right" size={18} />
            </Link>
          </section>
        </div>
      </main>

      <footer className="l-footer">
        <span>© {new Date().getFullYear()} Receh</span>
        <span>Catat. Rencanakan. Tenang.</span>
      </footer>
    </div>
  );
}
