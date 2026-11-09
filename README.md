# Receh: manajemen keuangan mahasiswa

Aplikasi web untuk mencatat dan merencanakan pemasukan/pengeluaran mahasiswa per dompet
(BCA, GoPay, DANA, tunai, dll.).

| Bagian | Teknologi | Dependensi pihak ketiga |
|---|---|---|
| `backend/` | Go 1.24, clean architecture, MySQL 8 | `github.com/go-sql-driver/mysql` (driver), `golang.org/x/crypto` (bcrypt) |
| `frontend/` | React 18 + TypeScript + Vite | `react`, `react-dom` (sisanya alat build) |

Router, chart, ikon, generator PDF, rate limiter, parser user-agent, cache, dan loader `.env` ditulis
sendiri, jadi tidak butuh library tambahan (tidak ada Redis, ORM, atau state manager).

---

## 1. Menjalankan aplikasi

### Prasyarat
- Go 1.24+ (`go version`)
- Node.js 18+ (`node -v`)
- **MySQL 8.0.16+** yang sedang berjalan (Laragon, XAMPP dengan MySQL 8, atau `docker run -p 3306:3306 -e MYSQL_ALLOW_EMPTY_PASSWORD=yes mysql:8.0`)

Koneksi database diatur di `backend/.env`:

```ini
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=
DB_NAME=receh
DB_MAX_OPEN_CONNS=20
# atau satu baris: DB_DSN=user:pass@tcp(host:3306)/receh
```

Database `DB_NAME` dan semua tabelnya **dibuat otomatis** saat server pertama kali jalan
(user MySQL perlu hak `CREATE`). Di produksi, sebaiknya buat user khusus, bukan `root`.

### Mode development (dua terminal)

```powershell
# Terminal 1: backend API di http://localhost:8080
cd backend
copy .env.example .env      # lalu sesuaikan DB_*; isi ADMIN_PASSWORD supaya mudah diingat
go run ./cmd/api
```

```powershell
# Terminal 2: frontend dengan hot reload di http://localhost:5173
cd frontend
npm install
npm run dev
```

Buka **http://localhost:5173**. Vite meneruskan `/api/*` ke Go, jadi browser melihat satu origin
dan cookie sesi langsung berfungsi.

### Mode produksi (satu proses)

```powershell
cd frontend; npm install; npm run build      # menghasilkan frontend/dist
cd ../backend; go build -o bin/receh.exe ./cmd/api
./bin/receh.exe                              # melayani UI + API di http://localhost:8080
```

### Akun admin pertama
Saat database masih kosong, server membuat satu admin:
- email: `ADMIN_EMAIL` (default `admin@receh.local`)
- sandi: `ADMIN_PASSWORD`. **Kalau kosong, sandi acak dicetak sekali di log server**, jadi simpan saat itu.

Pengguna biasa mendaftar sendiri di `/daftar` (tanpa OTP/email), lewat **Google**, atau dibuatkan oleh admin.

### Masuk dengan Google (opsional)

1. Di [Google Cloud Console](https://console.cloud.google.com/) → *APIs & Services* → *OAuth consent screen*:
   isi nama aplikasi; selama status *Testing*, tambahkan email penguji di *Test users*.
2. *Credentials* → *Create credentials* → *OAuth client ID* → tipe **Web application**.
   Di **Authorized redirect URIs** tambahkan URL yang Anda pakai (harus sama persis, termasuk port):
   - `http://localhost:5173/api/auth/google/callback` (mode development, lewat Vite)
   - `http://localhost:8080/api/auth/google/callback` (mode produksi lokal)
3. Isi `backend/.env` (file ini ada di `.gitignore`; client secret jangan di-commit):
   ```ini
   GOOGLE_CLIENT_ID=xxxxxxxx.apps.googleusercontent.com
   GOOGLE_CLIENT_SECRET=xxxxxxxx
   GOOGLE_REDIRECT_URL=http://localhost:5173/api/auth/google/callback
   ```
4. Jalankan ulang backend; log menulis `Masuk dengan Google aktif`. Tombol **Masuk dengan Google**
   muncul di `/masuk` dan `/daftar`. Kalau `GOOGLE_CLIENT_ID` kosong, tombol disembunyikan.

Buka aplikasi di origin yang sama dengan `GOOGLE_REDIRECT_URL` (misalnya `localhost:5173`, bukan
`127.0.0.1:5173`), karena cookie state hanya terkirim ke host yang memulai alur.

### Tes

```powershell
cd backend
go test ./...                       # tes unit + end-to-end HTTP (butuh MySQL)
$env:CGO_ENABLED=1; go test -race ./...   # sama, dengan race detector (butuh gcc)
```

Tes end-to-end membuat database sementara `receh_test_<acak>` untuk **setiap tes** lalu menghapusnya,
jadi tidak menyentuh database `receh`. Server MySQL yang dipakai: `root@tcp(127.0.0.1:3306)/`, atau
`TEST_DB_DSN` kalau di-set (contoh `$env:TEST_DB_DSN="user:pass@tcp(127.0.0.1:3306)/"`). Kalau MySQL
tidak terjangkau, tes end-to-end di-*skip* dengan pesan yang jelas.

Konfigurasi lain ada di `backend/.env.example`. Yang penting di produksi: `COOKIE_SECURE=true`
(HTTPS) dan `TRUST_PROXY=true` **hanya** kalau berada di belakang reverse proxy.

---

## 2. Fitur & pemetaan requirement

| Requirement | Di mana |
|---|---|
| Tambah wallet | Halaman **Dompet**: pilih cepat BCA/GoPay/DANA/OVO/…, isi saldo awal |
| Catat pemasukan/pengeluaran per wallet + nama kegiatan | Tombol **Catat transaksi** (atau tekan **N**) dari halaman mana pun |
| Saldo berubah sesuai wallet | Atomik di DB, dijaga `sync.Mutex` per dompet; saldo tidak bisa minus |
| Pindah uang antar dompet (+ biaya admin) | **Catat → Transfer**, tombol **Pindah uang** di Dompet, atau **Transfer** di Ringkasan. Lihat [bagian 3a](#3a-transfer--biaya-admin-keputusan) |
| Riwayat | **Riwayat transaksi**: cari, filter jenis/dompet/kategori/tanggal, ubah, hapus |
| Kategori | **Kategori**: 15 bawaan (termasuk *Biaya admin*) + tambah/ubah/hapus sendiri |
| Transaksi rutin | **Rutin & rencana**: harian/mingguan/bulanan/tahunan, tercatat otomatis saat jatuh tempo |
| Laporan bulanan PDF | **Laporan bulanan** → *Unduh PDF* |
| Pantau perangkat | **Perangkat & keamanan**: perangkat, lokasi, IP, waktu aktif, tombol keluarkan |
| Dashboard 2 kolom | Kolom 1: bar chart + pie chart interaktif dengan filter satuan waktu & dompet. Kolom 2: transaksi terbaru |
| Admin: tambah, tangguhkan, bekukan user | **Pengguna** (`/admin`) |
| Admin: audit log (lokasi, waktu, IP, perangkat) | **Audit log** (`/admin/audit`) |
| Landing page + registrasi tanpa OTP | `/` dan `/daftar` |
| Login dengan Google (OAuth) | Tombol di `/masuk` & `/daftar`; status di **Perangkat & keamanan → Metode masuk**. Lihat [bagian 3c](#3c-masuk-dengan-google-oauth-20--pkce) |
| Paket Gratis/Premium, hadiah Premium untuk keluarga, kode undangan | **Langganan** (pengguna), **Pengguna → Premium** dan **Kode undangan** (admin). Lihat [bagian 3-0](#3-0-edisi-branch-dan-paket-freemium) |
| Anggaran per kategori | **Anggaran**: batas bulanan, status aman / hampir habis / lewat, sisa per hari; kartu di dashboard; sisa anggaran tampil saat mencatat |
| Target tabungan | **Target tabungan**: setoran manual atau ikut saldo dompet khusus; setoran per minggu agar tepat waktu |
| Notifikasi & pengingat | Lonceng (8 jenis: anggaran, tagihan besok/gagal, saldo menipis, target tercapai, pengingat malam, ringkasan Senin); atur di **Notifikasi** |
| Pencatatan cepat | Kotak **Ketik cepat** di modal Catat ("makan siang 25rb gopay kemarin"), saran nama dari riwayat, **Impor mutasi** CSV (Premium) yang bisa dibatalkan |

**Tangguhkan vs bekukan:**
- **Ditangguhkan:** tidak bisa masuk sama sekali, semua sesi langsung dicabut, dan alasan ditampilkan saat mencoba login.
- **Dibekukan:** masih bisa masuk, melihat data, dan mengunduh PDF, tetapi semua perubahan ditolak (HTTP 403) dan transaksi rutin berhenti.

---

## 3. Arsitektur backend (clean architecture)

Dependensi hanya mengarah ke dalam: `delivery → usecase → domain ← repository`.
`domain` tidak meng-import apa pun dari lapisan luar.

```
backend/
├─ cmd/api/main.go                 Titik masuk: config → DB → app → HTTP server, graceful shutdown
└─ internal/
   ├─ domain/                      INTI: entitas, aturan, error, interface (port)
   │  ├─ entities.go               User, Wallet, Transaction, Recurring, AuditLog, read model dashboard/laporan
   │  ├─ errors.go                 AppError (kode stabil + pesan ramah); ErrInsufficient, ErrAccountFrozen, …
   │  ├─ ports.go                  Interface repository + GeoLocator + Clock
   │  └─ schedule.go               NextOccurrence: hitung jatuh tempo (tanggal 31 → 28 Feb → 31 Mar)
   ├─ usecase/                     PERILAKU APLIKASI (tanpa SQL, tanpa HTTP)
   │  ├─ auth.go                   Daftar, masuk, sesi, ganti/buat sandi, rate limit, buffer last-seen (mutex)
   │  ├─ oauth.go                  Masuk dengan Google: state + PKCE, cari/tautkan/buat akun, cek status & role
   │  ├─ audit.go                  AuditRecorder: tulis log langsung, cari lokasi di worker goroutine
   │  ├─ wallet.go                 CRUD dompet; dompet berisi riwayat diarsipkan, bukan dihapus
   │  ├─ category.go               CRUD kategori; tolak hapus kategori yang masih dipakai
   │  ├─ transaction.go            Validasi + kunci mutex dompet → repository (atomik)
   │  ├─ transfer.go               Pindah uang + biaya admin; kunci mutex kedua dompet; biaya terakhir per rute
   │  ├─ budget.go                 Anggaran bulanan; terpakai dihitung dari transaksi, bukan disimpan
   │  ├─ goal.go                   Target tabungan (setoran manual / ikut saldo dompet), mutex per target
   │  ├─ notification.go           Antrean channel + worker goroutine (peristiwa) + scheduler + worker pool (waktu)
   │  ├─ entry.go                  Ketik cepat, saran nama, pratinjau & impor CSV per batch, batalkan impor
   │  ├─ membership.go             Paket Gratis/Premium, hadiah admin, kode undangan
   │  ├─ recurring.go              Jadwal rutin + scheduler (ticker) + worker pool goroutine
   │  ├─ dashboard.go              Bucket per hari/minggu/bulan/tahun; query paralel; cache berversi
   │  ├─ report.go                 Kumpulkan data bulanan secara paralel → ReportRenderer (PDF)
   │  ├─ admin.go                  Tambah user, ubah status, audit log, admin pertama
   │  └─ common.go                 Validasi, kategori bawaan, helper parallel(), notifier, RealClock
   ├─ repository/mysql/            IMPLEMENTASI PORT dengan MySQL 8 (InnoDB)
   │  ├─ schema.sql                Tabel + CHECK(balance >= 0) + unique index anti double-posting
   │  ├─ db.go                     Open (buat DB jika belum ada, schema per statement, pool), inTx + retry deadlock
   │  ├─ transfers.go              Transfer + 2 leg + baris biaya dalam satu commit; saldo diubah urut id
   │  └─ users/sessions/wallets/categories/transactions/recurring/audit.go
   ├─ repository/cached/           DECORATOR CACHE di atas port (usecase tidak tahu ada cache)
   │  └─ cached.go                 Sesi (per token) & user (per id), dihapus seketika saat revoke/suspend/freeze
   ├─ delivery/http/               LAPISAN HTTP (tanpa aturan bisnis)
   │  ├─ router.go                 Tabel route (net/http 1.22+ pattern) + penyaji SPA frontend/dist
   │  ├─ middleware.go             Guard anyone/userRead/userWrite/admin, cookie, CSRF origin check,
   │  │                            header keamanan, log, recover panic, IP klien
   │  ├─ respond.go                JSON in/out, AppError → status HTTP
   │  ├─ handlers_oauth.go         /api/auth/providers, /google/start (302 ke Google), /google/callback
   │  └─ handlers_auth/finance/transfer/admin.go
   ├─ platform/                    ALAT TEKNIS KECIL YANG BISA DIPAKAI ULANG
   │  ├─ cache/                    TTL map (sync.RWMutex + goroutine pembersih) + versi data per user
   │  ├─ googleoauth/              Klien Google OIDC dengan net/http: URL otorisasi, tukar code, userinfo
   │  ├─ keymutex/                 sync.Mutex per kunci (per dompet), urut agar bebas deadlock
   │  ├─ ratelimit/                Fixed-window limiter berbasis sync.Mutex
   │  ├─ geoip/                    IP → kota (cache + mutex; IP lokal tidak dikirim keluar)
   │  ├─ useragent/                User-Agent → "Chrome di Android"
   │  └─ pdf/                      Penulis PDF minimal (pdf.go) + tata letak laporan (report.go)
   ├─ config/config.go             Baca env + .env, default
   └─ app/app.go                   Composition root: merakit semua lapisan; dipakai main & tes
```

### Alur satu permintaan: "catat pengeluaran Rp 25.000 dari GoPay"

```
React TransactionForm ──POST /api/transactions──► middleware: recover → log → header → sameOrigin
   ► router: userWrite guard (cookie → AuthService.Authenticate → cek role & status beku)
        (sesi & user diambil dari cache memori; miss → MySQL)
   ► handlers_finance.createTransaction (decode JSON)
   ► usecase.TransactionService.Create
        1. validasi (nominal, nama, tanggal, dompet & kategori milik user, jenis kategori cocok)
        2. locks.Lock(walletID)              ← sync.Mutex khusus dompet ini
        3. repo.Create → BEGIN (InnoDB)
               UPDATE wallets SET balance = balance - 25000 WHERE id=? AND balance - 25000 >= 0
               (0 baris = saldo kurang → ErrInsufficient → HTTP 409; baris dompet terkunci sampai commit)
               INSERT INTO transactions …
           COMMIT   (deadlock/lock timeout → diulang otomatis maks. 3×)
        4. unlock, lalu versi data user naik → cache dashboard user ini tidak terpakai lagi
   ◄ 201 JSON ── FinanceProvider.changed() → dashboard, saldo, riwayat memuat ulang
```

### 3-0. Edisi, branch, dan paket (freemium)

| Branch | Untuk | `BILLING_ENABLED` |
|---|---|---|
| `main` | Arsip versi pertama | — |
| `b2c` | **Semua pengembangan baru**; versi yang dijual (freemium) | `true` (default) |
| `personal` | Pemakaian pribadi; menerima `git merge b2c` | `false` → semua akun Premium |

Satu kode, beda mode: `usecase/membership.go` (`MembershipService`) adalah satu-satunya tempat yang
menjawab "apakah pengguna ini Premium?". Use case lain bertanya lewat interface `PlanLimits`
(contoh: `WalletService.Create` menolak dompet ke-4 di paket Gratis dengan kode `PLAN_LIMIT`).

| | Gratis | Premium |
|---|---|---|
| Dompet aktif | 3 | Tanpa batas |
| Anggaran per kategori | 3 | Tanpa batas |
| Target tabungan | 1 | Tanpa batas |
| Impor mutasi CSV | — | ✓ |

Premium didapat dari tabel `entitlements` (riwayat tidak pernah dihapus; pencabutan mengisi `revoked_at`):
- **Hadiah admin** (*Admin → Pengguna → Premium*): selamanya atau sampai tanggal, dengan catatan
  (mis. "Keluarga"), bisa dicabut, tercatat di audit log.
- **Kode undangan** (*Admin → Kode undangan*): kuota, durasi (hari/selamanya), batas tanggal penukaran.
  Penukaran mengunci baris kode (`SELECT … FOR UPDATE`), sehingga kuota tidak bisa terlewati walau
  ditukar bersamaan (diuji 30 penukaran paralel, kuota 5 → tepat 5 berhasil); satu akun satu kali per kode
  (unique index); 10 percobaan salah per jam per akun.
- **Berbayar**: disiapkan (`source='paid'`), payment gateway menyusul.

Dengan `BILLING_ENABLED=false`, menu Langganan disembunyikan, semua batas hilang, dan admin tetap bisa
melihat datanya.

### 3a. Transfer & biaya admin (keputusan)

Contoh: top up GoPay Rp 100.000 dari BCA, biaya admin Rp 1.000.

| Keputusan | Alasan |
|---|---|
| **Biaya ditanggung dompet asal.** BCA −Rp 101.000, GoPay +Rp 100.000 | Begitulah m-banking/e-wallet memotongnya; saldo di aplikasi cocok dengan saldo asli |
| **Transfer bukan pemasukan/pengeluaran** | Uangnya tetap milikmu; kalau dihitung, chart "pengeluaran" membengkak palsu |
| **Biaya admin = pengeluaran sungguhan**, kategori *Biaya admin* | Uang itu benar-benar hilang; jadi terlihat di pie chart, riwayat, dan PDF ("bulan ini habis Rp 12.500 untuk biaya admin") |
| **Tarif tidak di-hardcode per bank.** Form mengisi otomatis **biaya terakhir untuk rute yang sama**, plus chip cepat Gratis / 1.000 / 2.500 / 6.500 | Tarif bergantung pada pasangan asal–tujuan (BCA→BCA gratis, BCA→GoPay ±1.000, BI-FAST 2.500, online 6.500) dan bisa berubah. Tabel tarif buatan sendiri akan cepat salah; riwayat pengguna sendiri selalu relevan |
| Transfer bisa **diubah & dihapus**; leg + biaya ikut dibalik dalam satu commit | Salah ketik nominal itu biasa; saldo tidak boleh setengah terbalik |

Di database satu transfer = 1 baris `transfers` + leg `transfer_out` (dompet asal) + leg `transfer_in`
(dompet tujuan) + 1 pengeluaran biaya (kalau > 0), semuanya punya `transfer_id`. Query chart hanya
menjumlah `income`/`expense`, jadi leg transfer otomatis tidak terhitung. Riwayat tanpa filter dompet
menampilkan satu baris "BCA → GoPay"; difilter ke GoPay, leg masuknya yang tampil. Baris transfer tidak
bisa diubah satu per satu lewat endpoint transaksi (HTTP 409), selalu lewat transfernya.

### 3b. Caching (keputusan: di backend **dan** frontend, masing-masing untuk masalah berbeda)

| Lapisan | Apa yang di-cache | Kenapa di sini | Bagaimana tetap benar |
|---|---|---|---|
| Backend `repository/cached` | Sesi (per hash token) & user (per id), TTL `SESSION_CACHE_TTL`=30s | Dibaca di **setiap** request; dengan MySQL lewat jaringan, ini 2 query per request | Logout, cabut perangkat, suspend, freeze, ganti sandi **menghapus entri seketika**. Penghitung *epoch* mencegah request yang sedang membaca DB menaruh data lama kembali ke cache sesudah dicabut |
| Backend `usecase/dashboard.go` | Hasil dashboard per (user, unit, offset, dompet, hari ini), TTL `DASHBOARD_CACHE_TTL`=60s | 6 query agregasi; berpindah Harian/Mingguan/periode sering mengulang permintaan yang sama | Kunci cache memuat **versi data user**. Setiap penulisan (transaksi, transfer, dompet, kategori, jadwal rutin, termasuk scheduler) menaikkan versi, jadi data lama tidak pernah tersaji |
| Frontend `lib/hooks.ts` | Respons GET terakhir per endpoint, di memori tab | Kembali ke halaman/periode yang sama langsung tampil tanpa skeleton | *Stale-while-revalidate*: selalu tetap meminta data baru; dikosongkan saat logout/ganti akun; tidak pernah ke localStorage (data keuangan) |

### 3c. Masuk dengan Google (OAuth 2.0 + PKCE)

```
Browser: klik "Masuk dengan Google" (<a href>, navigasi penuh)
  ► GET /api/auth/google/start                         handlers_oauth.googleStart
       AuthService.StartGoogle: state & verifier acak (32 byte), challenge = SHA-256(verifier)
       state → verifier disimpan di cache server (10 menit); cookie receh_oauth = state (HttpOnly, Lax)
  ◄ 302 accounts.google.com?client_id&redirect_uri&scope=openid email profile&state&code_challenge (S256)
Google: pilih akun ──► 302 /api/auth/google/callback?code&state
  ► handlers_oauth.googleCallback → AuthService.FinishGoogle
       1. state di URL == cookie, lalu Take(state) dari cache (sekali pakai, atomik)   ← anti login-CSRF & replay
       2. googleoauth.Exchange: POST token endpoint (client_secret + code_verifier) → GET userinfo
       3. email_verified wajib true
       4. google_sub dikenal → akun itu | email sama → tautkan | belum ada → akun baru + kategori bawaan
       5. admin → tolak; ditangguhkan → tolak + alasan; dibekukan → boleh masuk (read-only)
       6. startSession + audit log (perangkat, IP, lokasi) seperti login biasa
  ◄ 302 /app  (akun baru: /app/dompet?baru=1)   |   gagal: 302 /masuk?oauth=<KODE>
```

| Keputusan | Alasan |
|---|---|
| Alur *authorization code* di server + PKCE + state | Client secret dan verifier tidak pernah sampai ke browser |
| Identitas dari endpoint *userinfo*, bukan dari JWT | Token didapat langsung dari Google lewat TLS; tidak perlu library JWT |
| Tautkan otomatis ke akun berpassword dengan email yang sama | Email sudah diverifikasi Google. Satu akun Google hanya bisa tertaut ke satu akun, dan tautan tidak pernah ditimpa |
| Admin tidak bisa masuk lewat Google | Akun paling berkuasa cukup punya satu pintu |
| Akun Google bisa **membuat kata sandi** tanpa sandi lama | Supaya tetap bisa masuk tanpa Google; setelah itu ganti sandi seperti biasa |
| Alasan penangguhan dikirim lewat cookie 1 menit, bukan URL | Tautan `/masuk?…` buatan orang lain tidak bisa menampilkan teks palsu di halaman kita |

Yang sengaja **tidak** di-cache: respons API di browser (`no-store`, data pribadi), laporan PDF, dan
semua penulisan. **Tanpa Redis:** aplikasi berjalan sebagai satu proses, dan Redis menambah satu server
untuk dioperasikan. Kalau nanti dijalankan lebih dari satu instance, cache memori harus diganti cache
bersama (atau dimatikan dengan TTL `0`), karena pencabutan sesi di instance A tidak terlihat oleh instance B.

### Di mana `sync.Mutex` dan goroutine dipakai, dan untuk apa

| Tempat | Mekanisme | Mencegah / tujuan |
|---|---|---|
| `platform/keymutex` | `sync.Mutex` per dompet, diambil terurut | Dua pengeluaran bersamaan dari dompet yang sama membuat saldo minus atau update hilang. Dompet berbeda tetap paralel |
| `usecase/recurring.go` `runMu` | `sync.Mutex` | Dua sweep scheduler berjalan bersamaan |
| `usecase/recurring.go` `RunDue` | worker pool goroutine + `sync.WaitGroup` + `atomic` | Memproses banyak jadwal jatuh tempo secara paralel |
| `usecase/recurring.go` `RunScheduler` | goroutine + `time.Ticker` | Mencatat transaksi rutin tiap jam dan mengejar ketertinggalan saat server mati |
| `usecase/audit.go` | channel + 2 worker goroutine | Lookup lokasi yang lambat tidak menunda login |
| `usecase/auth.go` `touchMu` | `sync.Mutex` + goroutine `Maintain` | Buffer "terakhir aktif" ditulis massal tiap 30 detik, bukan per request |
| `usecase/common.go` `parallel()` | goroutine + `WaitGroup` + `Mutex` (error pertama) | Query dashboard dan laporan berjalan bersamaan |
| `platform/ratelimit`, `platform/geoip` | `sync.Mutex` | Map bersama diakses banyak request sekaligus |
| `usecase/transfer.go` | `keymutex.Lock(asal, tujuan)` (2–4 kunci saat edit) | Transfer A→B dan B→A bersamaan tidak deadlock dan tidak bisa membuat salah satu minus |
| `platform/cache` `TTL` | `sync.RWMutex` + goroutine janitor per cache | Banyak request membaca cache bersamaan, satu menulis; entri kedaluwarsa dibersihkan tiap menit |
| `platform/cache` `Versions` | `sync.Mutex` | Versi data per user dinaikkan dari request dan scheduler sekaligus |
| `repository/cached` epoch | `sync/atomic` | Invalidasi yang berbarengan dengan pembacaan DB tidak "hilang" |
| `usecase/notification.go` | channel + worker goroutine; ticker + pool 4 goroutine | Notifikasi anggaran muncul sesaat setelah belanja tanpa memperlambat request; penyapuan berkala untuk tagihan besok, pengingat malam, ringkasan Senin |
| `usecase/goal.go` `perGoal` | `keymutex` per target | Dua penarikan bersamaan tidak membuat tabungan minus (diuji 20 paralel) |
| `platform/cache` `Take` | `sync.RWMutex` (ambil+hapus atomik) | State OAuth tidak bisa dipakai dua kali |

Lapisan kedua pertahanan ada di database: `CHECK (balance >= 0)`, `UPDATE … WHERE balance + ? >= 0`
(InnoDB mengunci baris dompet sampai commit), saldo transfer diubah urut id dompet agar urutan kunci
baris selalu sama, dan unique index `(recurring_id, occurred_on)`, sehingga jadwal rutin tidak pernah
tercatat dua kali walau scheduler dan request user berbenturan.

Tes konkurensi (semua di MySQL sungguhan):
- `TestConcurrentSpendingNeverOverdrawsOrLosesUpdates`: 100 pengeluaran Rp 1.000 bersamaan ke dompet berisi Rp 50.000 → tepat 50 berhasil, saldo akhir tepat 0.
- `TestConcurrentTransfersBothWaysStayConsistent`: 50 transfer A→B dan 50 B→A bersamaan → semua berhasil, tanpa deadlock, total saldo = awal − total biaya.
- `TestConcurrentTransfersNeverOverdraw`: 40 transfer Rp 10.000 + biaya Rp 1.000 dari dompet Rp 55.000 → tepat 5 berhasil, saldo 0.

---

## 4. Arsitektur frontend

```
frontend/
├─ index.html                 Font (Bricolage Grotesque, Plus Jakarta Sans, DM Mono), theme-init.js
├─ public/theme-init.js       Pasang tema tersimpan sebelum render (CSP melarang script inline)
├─ vite.config.ts             Proxy /api → :8080 saat dev
└─ src/
   ├─ main.tsx                Provider: Toast → Confirm → Auth → Finance → App
   ├─ App.tsx                 Tabel route + guard role (/app untuk user, /admin untuk admin)
   ├─ styles.css              DESIGN SYSTEM: token warna terang/gelap, komponen, layout responsif
   ├─ lib/
   │  ├─ api.ts               fetch wrapper; ApiError; event global (401 → logout, 403 beku → refresh)
   │  ├─ auth.tsx             AuthProvider: user saat ini, login/logout
   │  ├─ finance.tsx          FinanceProvider: cache dompet+kategori, `version` untuk refetch, modal transaksi global
   │  ├─ router.ts            Router ±30 baris di atas History API (useSyncExternalStore)
   │  ├─ hooks.ts             useFetch + cache stale-while-revalidate, useDebounced, useStoredState
   │  ├─ format.ts            Rupiah, tanggal Indonesia, label
   │  └─ types.ts             Tipe yang mencerminkan JSON backend
   ├─ components/
   │  ├─ AppShell.tsx         Sidebar (desktop) / top bar + bottom nav dengan tombol + (mobile), tema, banner beku, pintasan N
   │  ├─ GoogleButton.tsx     Tombol Google (tampil hanya bila aktif), pemisah "atau", pesan ?oauth=
   │  ├─ TransactionForm.tsx  Modal global: Pengeluaran / Pemasukan / Transfer; form catat/ubah transaksi
   │  ├─ TransferForm.tsx     Dari → Ke (tombol tukar), nominal, biaya admin (terisi dari rute terakhir),
   │  │                       pratinjau saldo kedua dompet sebelum & sesudah, ubah/hapus transfer
   │  ├─ charts.tsx           BarChart & PieChart SVG: hover, klik, keyboard, label aksesibel
   │  ├─ TxRow.tsx            Baris transaksi (dipakai dashboard, riwayat)
   │  ├─ ui.tsx               Modal (<dialog>), Toast, Confirm, Field, AmountInput, ColorPicker, Empty, …
   │  └─ Icon.tsx             Ikon garis inline + logo
   └─ pages/                  Landing, Auth (Masuk/Daftar), Dashboard, Transactions, Wallets,
                              Categories, Recurring, Reports, Security, AdminUsers, AdminAudit
```

**Interaksi data:** halaman mengambil data lewat `useFetch('/…?v=' + version)`. Setiap aksi tulis
(transaksi, dompet, jadwal) memanggil `changed()` di `FinanceProvider`, yang menaikkan `version`.
Semua halaman yang sedang terbuka lalu memuat ulang, jadi saldo di sidebar form, dashboard, dan
riwayat selalu konsisten tanpa state management tambahan.

### Keputusan UI/UX
- **Paham sekali lihat:** saldo total sebagai angka besar, struk transaksi terbaru di kolom kanan,
  serta warna hijau (masuk) dan oranye (keluar) yang konsisten di semua tempat.
- **Cepat mencatat:** satu modal global (tombol, **N**, atau + di mobile); nominal otomatis fokus dan diformat ribuan;
  dompet & kategori berupa chip satu ketukan; sisa saldo dihitung saat mengetik.
- **Chart interaktif tapi jujur:** klik batang memfokuskan pie ke periode itu; klik baris kategori membuka riwayat terfilter;
  setiap chart punya padanan tabel; pie dilipat jadi ≤6 irisan ("Kategori lain").
- **Warna kategori** diambil dari palet 8 slot yang **sudah divalidasi aman untuk buta warna** di tema terang dan
  gelap, dan disimpan sebagai slot (`c1`…`c8`), bukan hex, sehingga tiap tema memakai versinya sendiri.
- **Aman dari salah klik:** semua penghapusan lewat dialog konfirmasi; dompet berisi riwayat diarsipkan.
- **Aksesibilitas:** label form, fokus terlihat, `<dialog>` native (fokus terkunci, Esc), toast `aria-live`,
  `prefers-reduced-motion`, target sentuh ≥ 34px.
- **Responsif:** sidebar di desktop, bottom nav di ≤ 860px, modal menjadi bottom sheet di ≤ 560px.

---

## 5. Keamanan singkat
- Sandi di-hash bcrypt; token sesi acak 256-bit, hanya hash SHA-256 yang disimpan; cookie `HttpOnly` + `SameSite=Lax`.
- CSRF: request non-GET dengan `Origin` dari situs lain ditolak.
- Rate limit login (8 gagal/10 menit per IP+email) dan pendaftaran (20/jam per IP).
- Waktu respons login sama untuk email terdaftar dan tidak (bcrypt dummy); alasan penangguhan hanya
  ditampilkan setelah sandi benar.
- Ganti sandi mengeluarkan semua perangkat lain; penangguhan mencabut semua sesi.
- Google: PKCE (S256) + state sekali pakai yang terikat ke cookie browser; client secret hanya di server;
  email harus terverifikasi Google; akun tanpa sandi ditolak saat login sandi dengan waktu respons yang sama.
- `X-Forwarded-For` diabaikan kecuali `TRUST_PROXY=true`, sehingga IP di audit log tidak bisa dipalsukan.
- Lokasi: IP publik dikirim ke layanan `GEOIP_URL` (default `ipwho.is`); IP lokal/privat tidak pernah dikirim.
  Kosongkan `GEOIP_URL` untuk mematikan.

## 6. Batasan yang diketahui
- Cache backend ada di memori satu proses (lihat [3b](#3b-caching-keputusan-di-backend-dan-frontend-masing-masing-untuk-masalah-berbeda)); untuk beberapa instance, matikan (`SESSION_CACHE_TTL=0`, `DASHBOARD_CACHE_TTL=0`) atau ganti dengan cache bersama.
- Transfer rutin (misalnya top up GoPay otomatis tiap bulan) belum ada; jadwal rutin hanya untuk pemasukan/pengeluaran.
- Data dari versi SQLite sebelumnya tidak dimigrasikan.
- Google belum bisa dilepas (*unlink*) dari akun lewat UI, dan state OAuth disimpan di memori satu proses
  (alur yang sedang berjalan saat server di-restart harus diulang).
- PDF memakai font bawaan Helvetica, jadi karakter di luar Latin (emoji, aksara non-Latin) tercetak sebagai "?".
- Admin pertama tidak bisa diubah statusnya oleh admin, dan admin tidak punya fitur keuangan.

## 7. Fase 1 "layak jual": catatan desain singkat

- **Anggaran** tidak menyimpan angka "terpakai"; selalu dihitung dari transaksi bulan itu, jadi edit/hapus transaksi dan biaya transfer otomatis tercermin.
- **Target tabungan** punya dua mode. *Setoran manual* = celengan virtual (catatan alokasi, saldo dompet tidak berubah). *Dompet khusus* = progres mengikuti saldo dompet; menabung cukup dengan Transfer.
- **Notifikasi**: setiap penulisan data memanggil `ChangeNotifiers` (versi cache dashboard + antrean notifikasi). Event yang sama tidak pernah dikirim dua kali karena `ref_key` unik per pengguna. Notifikasi lebih dari 90 hari dihapus otomatis.
- **Ketik cepat** adalah fungsi murni `domain.ParseQuick` (diuji dengan tabel kalimat); bila teks tidak menyebut dompet/kategori, dipakai yang terakhir untuk nama yang sama.
- **Impor CSV** (`domain.ParseStatement`) mengenali format BCA (`DD/MM`, `DB`/`CR`), kolom Debit/Kredit e-wallet, angka `1.250.000,00` maupun `1,250,000.00`, dan file tanpa header. Satu impor = satu batch: saldo bergerak sekali sebesar selisih bersih, dan bisa dibatalkan sekaligus.
