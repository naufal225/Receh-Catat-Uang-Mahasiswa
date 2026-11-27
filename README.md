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

### Dengan Docker (paling mudah, edisi pribadi)

Hanya butuh [Docker Desktop](https://www.docker.com/products/docker-desktop/) (atau Docker Engine +
Compose di Linux). Go, Node, dan MySQL tidak perlu dipasang.

```bash
git clone -b personal https://github.com/naufal225/Receh-Catat-Uang-Mahasiswa.git
cd Receh-Catat-Uang-Mahasiswa
docker compose up -d --build
docker compose logs app        # sandi admin pertama dicetak di sini (sekali saja)
```

Buka **http://localhost:8080**, lalu daftar akun di `/daftar`. Semua fitur Premium aktif tanpa batas
(`BILLING_ENABLED=false`). Akun admin hanya untuk mengelola pengguna.

| Perintah | Fungsi |
|---|---|
| `docker compose stop` / `docker compose start` | Matikan / nyalakan lagi |
| `git pull && docker compose up -d --build` | Perbarui ke versi terbaru |
| `docker compose down` | Hapus container; data tetap tersimpan |
| `docker compose down -v` | Hapus container **beserta seluruh data** |

Pengaturan opsional (port, sandi admin, Google) ada di `.env.docker.example`; salin menjadi `.env`.

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

## 3. Arsitektur backend (clean architecture + modul)

Kode dibagi dua: **inti** (akun, sesi, Google, admin, audit, paket/limit, notifikasi, cache)
dan **modul fitur** di `internal/modules/<nama>`. Inti tidak pernah meng-import modul;
modul memakai inti lewat `modules.Host`. Modul keuangan adalah contoh yang bisa dicopot.
Di dalam inti maupun modul, dependensi tetap mengarah ke dalam: `api → service → model ← store`.

```
backend/
├─ cmd/api/main.go                 Titik masuk: config → DB → app (migrasi modul) → HTTP server, graceful shutdown
└─ internal/
   ├─ domain/                      INTI: User, Session, AuditLog, paket (LimitDef/Limits/Usage), notifikasi, error, port
   ├─ usecase/                     PERILAKU INTI (tanpa SQL, tanpa HTTP)
   │  ├─ org.go                    Workspace: peran, anggota, undangan (token ter-hash), cache peran, audit
   │  ├─ auth.go / oauth.go        Daftar, masuk, sesi, Google (state + PKCE), rate limit
   │  ├─ common.go                 UserHooks (modul menyiapkan akun baru), validasi, RealClock
   │  ├─ membership.go             Paket Gratis/Premium; limit & penghitung pemakaian didaftarkan modul
   │  ├─ notification.go           NotificationSource dari modul + antrean channel + worker + scheduler + inbox/prefs
   │  ├─ admin.go / audit.go       Kelola user, audit log (lokasi dicari di worker goroutine)
   ├─ kit/                         Helper bersama inti & modul: Parallel, CleanName, FormatRp, CheckLimit, Notifier
   ├─ repository/mysql/            Store inti + helper ekspor (InTx, ApplySchema, Migrate, …) untuk store modul
   ├─ repository/cached/           Decorator cache sesi & user
   ├─ delivery/http/               Router inti, guard, JSON; Server.Mount(...) untuk rute modul
   ├─ modules/
   │  ├─ module.go                 Kontrak Module {Name, Migrate, Init} dan Host (DB, Clock, Locks, Changes, Plans, …)
   │  └─ finance/                  MODUL CONTOH
   │     ├─ module.go              Init: rakit service, daftarkan limit, usage, notifikasi, hook user baru, rute, job
   │     ├─ model/                 Entitas & port keuangan, ParseQuick, ParseStatement, NextOccurrence
   │     ├─ service/               Dompet, transaksi, transfer, anggaran, target, rutin, laporan, impor, notifikasi
   │     ├─ store/                 MySQL + schema.sql (ter-embed) + migrasi kolom milik modul
   │     ├─ api/                   Handler HTTP + routes.go
   │     └─ report/                Tata letak laporan PDF bulanan
   ├─ platform/                    cache, keymutex, ratelimit, geoip, googleoauth, payment/{mock,midtrans,xendit}, useragent, pdf
   ├─ config/                      Env + .env (APP_NAME, DB_*, BILLING_ENABLED, GOOGLE_*, …)
   └─ app/                         Composition root (app.go) + daftar modul (modules.go)
```

### 3-M. Membuat atau mencopot modul

**Mencopot modul keuangan:** hapus `backend/internal/modules/finance`, baris `finance.New()` di
`backend/internal/app/modules.go`, folder `frontend/src/modules/finance`, dan baris `finance` di
`frontend/src/modules/index.ts`. Inti tetap build dan tesnya hijau (`TestCoreRunsWithoutModules`
menjalankan aplikasi tanpa modul pada database kosong). Tabel modul di database lama tidak dihapus otomatis.

**Membuat modul baru** (`internal/modules/<nama>`): implementasikan `modules.Module`.
- `Migrate(db)`: `mysql.ApplySchema(db, schemaSQL)` + `mysql.Migrate(db, []mysql.Migration{...})`. Idempoten.
- `Init(h)`: rakit service dari `h.DB`, `h.Clock`, `h.Locks`, lalu daftarkan yang perlu saja:
  - `h.Membership.AddLimits(domain.LimitDef{Key: "<nama>.items", Free: 5, Premium: domain.Unlimited})`
    dan `AddUsageCounter(...)`; cek di service dengan `kit.CheckLimit` / `kit.RequireFeature` lewat `h.Plans`.
  - `h.Notifications.AddSource(src)` (jenis, pengaturan, dan `Evaluate`; dedup lewat `RefKey`).
  - `h.Orgs.OnOrgCreated(fn)` untuk data awal tiap workspace baru (pribadi maupun tim).
  - Kunci tabel modul dengan `org_id` (FK ke `organizations`) dan baca workspace permintaan dengan
    `core.OrgFrom(r.Context())`; guard `Read`/`Write` memastikan pemanggil anggota workspace itu.
  - `h.Server.Mount(func(mux, g) { mux.HandleFunc("GET /api/<nama>", g.Read(handler)) })`.
  - `h.Background(job)` untuk goroutine yang berjalan selama aplikasi hidup.
  - Setelah menulis data pengguna, panggil `h.Changes.Changed(userID)` (cache & notifikasi ikut segar).
- Frontend: buat `src/modules/<nama>/index.tsx` bertipe `AppModule` (`pages`, `nav`, opsional
  `Provider`, `Overlay`, `primary`) lalu tambahkan ke `src/modules/index.ts`.

### 3-O. Workspace (organisasi)

Data modul milik **workspace**, bukan pengguna. Setiap akun pengguna otomatis punya satu workspace
pribadi (tidak bisa diundangi anggota); tim adalah workspace tambahan.

| Peran | Boleh |
|---|---|
| owner (tepat satu) | semuanya: hapus tim, pindahkan kepemilikan, ubah peran, memegang langganan |
| admin | mengundang anggota biasa, mengeluarkan anggota biasa, ganti nama tim |
| member | memakai data tim (modul menentukan sisanya) |

- **Memilih workspace:** frontend mengirim header `X-Org-Id`. Server memeriksa keanggotaan di setiap
  permintaan, jadi header hanya pilihan, bukan izin. Tanpa header = workspace pribadi.
  Guard `Read`/`Write` untuk modul sudah memasangnya; ambil dengan `core.OrgFrom(r.Context())`.
- **Paket:** workspace memakai paket **owner**-nya (`MembershipService.Limit(orgID, key)`). Anggota
  non-owner melihat batas milik owner. Langganan per workspace datang di Fase C.
- **Undangan:** per email, berlaku 7 hari, sekali pakai. Hanya hash token yang disimpan; link
  (`/undangan?t=…`) tampil sekali ke pengundang. Hanya akun dengan email yang diundang yang bisa
  membukanya (selain itu "tidak ditemukan"). Penerimaan diserialkan dengan `SELECT … FOR UPDATE`,
  jadi dua orang yang membuka link yang sama tidak bisa sama-sama masuk. Pengiriman email datang di Fase D.
- **Notifikasi tim:** semua anggota mendapat notifikasi data tim, judulnya diawali `[Nama Tim]`.
- **Cache peran:** peran dicache 30 detik per (workspace, pengguna) dan dihapus seketika saat peran
  berubah atau anggota keluar, jadi akses hilang langsung.

**Upgrade database lama (aman untuk data asli).** Saat aplikasi pertama kali berjalan dengan skema ini:
1. Setiap akun yang belum punya workspace dibuatkan workspace pribadi (hanya menambah baris).
2. Tabel modul (dompet, kategori, transaksi, dst.) mendapat kolom `org_id`, diisi dari workspace
   pribadi pemilik lamanya; kolom `user_id` **tidak dihapus**, hanya diganti nama menjadi
   `legacy_user_id` (boleh NULL, tanpa constraint) sebagai catatan.
3. Setiap langkah memeriksa skema dulu, jadi migrasi yang terputus melanjutkan di start berikutnya
   dan start kedua tidak mengubah apa pun. Bila ada baris yang pemiliknya tak punya workspace,
   migrasi berhenti dengan pesan jelas (tanpa menghapus apa pun).

DDL MySQL tidak bisa di-rollback, jadi **backup dulu** sebelum start pertama pada data nyata:
`mysqldump -uroot receh > receh_backup.sql`. Tes `TestUpgradeFromBeforeWorkspacesKeepsEveryRow`
menjalankan seluruh jalur ini pada skema lama (fixture di `backend/internal/app/testdata`).

### Alur satu permintaan: "catat pengeluaran Rp 25.000 dari GoPay"

```
React TransactionForm ──POST /api/transactions──► middleware: recover → log → header → sameOrigin
   ► router: userWrite guard (cookie → AuthService.Authenticate → cek role & status beku)
        (sesi & user diambil dari cache memori; miss → MySQL)
   ► finance/api handlers_finance.createTransaction (decode JSON)
   ► service.TransactionService.Create (modul finance)
        1. validasi (nominal, nama, tanggal, dompet & kategori milik user, jenis kategori cocok)
        2. locks.Lock(walletID)              ← sync.Mutex khusus dompet ini
        3. repo.Create → BEGIN (InnoDB)
               UPDATE wallets SET balance = balance - 25000 WHERE id=? AND balance - 25000 >= 0
               (0 baris = saldo kurang → model.ErrInsufficient → HTTP 409; baris dompet terkunci sampai commit)
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
- **Berbayar** (`source='paid'`): lewat Midtrans, Xendit, atau gateway tiruan; lihat 3-P.

Dengan `BILLING_ENABLED=false`, menu Langganan disembunyikan, semua batas hilang, dan admin tetap bisa
melihat datanya.

### 3-P. Pembayaran (billing)

Paket **Pro** dibeli per **workspace** oleh **owner**-nya (admin dan anggota hanya melihat status).
Katalog ada di `backend/internal/config/catalog.go` (data biasa: harga, label, hari tenggang), jadi
pembeli template mengubah harga tanpa menyentuh logika. ID paket di database tetap `free`/`premium`;
nama "Pro" hanya tampilan.

| Hal | Perilaku |
|---|---|
| Harga | Pro bulanan Rp19.000, tahunan Rp190.000 (bayar 10 bulan, dapat 12) |
| Periode | **Sekali bayar per periode**, bukan tagihan otomatis. Bayar lebih awal menumpuk dari akhir periode berjalan, jadi sisa hari tidak hilang |
| Masa tenggang | 7 hari setelah periode habis: fitur Pro tetap terbuka, lalu otomatis kembali ke Gratis (data tidak dihapus; hanya membuat yang baru ditolak) |
| Pengingat | Notifikasi ke owner pada H-7, saat masa tenggang, dan saat berakhir (satu kali per periode) |
| Komplimen & kode | Tetap berlaku (berlaku untuk akun, `entitlements.org_id` NULL); pembayaran berlaku untuk satu workspace (`org_id` terisi). Yang paling lama menang |
| Pindah owner | Workspace berbayar tetap Pro; owner baru yang mengelola |

**Alur.** `POST /api/billing/checkout {priceId}` membuat satu *payment* `pending` per workspace
(dikunci lewat baris `organizations`, jadi dua klik bersamaan tidak membuat dua), memanggil gateway,
dan mengembalikan `checkoutUrl`. Gateway memanggil `POST /api/billing/webhook/{provider}`. Webhook **hanya petunjuk**:
tanda tangan diverifikasi, lalu server bertanya langsung ke gateway (`Fetch`) dan mencocokkan jumlah; baru
setelah itu `Settle` (satu transaksi: kunci org → payment → subscription) mengaktifkan periode dan membuat entitlement.
Event yang sama dua kali (`webhook_events`, unik per `provider+event_id`) tidak berefek. Pembayaran yang tiba
setelah checkout kedaluwarsa tetap dihormati (uangnya sudah masuk) dan dicatat. Jumlah yang tidak cocok tidak
memberi apa pun dan masuk audit (`payment_mismatch`). Webhook yang hilang ditutup `BillingService.Sweep`
(tiap 10 menit) dan tombol "Saya sudah bayar".

| Provider | `BILLING_PROVIDER` | Catatan |
|---|---|---|
| Mock | `mock` (default) | Halaman bayar tiruan di aplikasi; menolak jalan bila `COOKIE_SECURE=true` kecuali `MOCK_ALLOW_PROD=true` |
| Midtrans | `midtrans` | Snap (`redirect_url`, tanpa JS popup), status lewat Core API; tanda tangan `SHA512(order_id+status_code+gross_amount+ServerKey)` |
| Xendit | `xendit` | Invoice API v2; header `x-callback-token` dibandingkan dengan token dashboard (waktu konstan). **Belum dicocokkan dengan dokumentasi terbaru**, uji di sandbox (lihat `docs/SETUP-CHECKLIST.md`) |

Tanpa SDK: adaptor memakai `net/http` (`internal/platform/payment/{mock,midtrans,xendit}`), base URL bisa
diganti sehingga tes memakai server tiruan. Menambah gateway = implementasi `domain.PaymentProvider`
(`Name`, `CreateCheckout`, `Cancel`, `Fetch`, `ParseWebhook`) lalu satu `case` di `cmd/api/main.go`.

Konfigurasi: `BILLING_PROVIDER`, `PUBLIC_BASE_URL`, `MIDTRANS_SERVER_KEY`, `MIDTRANS_ENV`, `XENDIT_SECRET_KEY`,
`XENDIT_CALLBACK_TOKEN` (lihat `backend/.env.example`; kunci hanya di `backend/.env`). Provider yang dipilih tapi kuncinya
kosong → server menolak start dengan pesan jelas. Halaman admin **Pembayaran** menunjukkan kunci mana yang terisi (ya/tidak,
tidak pernah nilainya) dan URL webhook untuk ditempel ke dashboard penyedia. Di luar cakupan: tagihan otomatis (kartu tersimpan)
dan refund otomatis (lakukan lewat dashboard penyedia, lalu cabut Pro dari halaman admin bila perlu).

Tabel baru: `payments` (salinan nama workspace dan email pembayar, bertahan walau workspace/akun dihapus),
`subscriptions`, `webhook_events`; kolom baru `entitlements.org_id` (migrasi aditif, data lama tidak berubah).

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
| Backend `finance/service/dashboard.go` | Hasil dashboard per (user, unit, offset, dompet, hari ini), TTL `DASHBOARD_CACHE_TTL`=60s | 6 query agregasi; berpindah Harian/Mingguan/periode sering mengulang permintaan yang sama | Kunci cache memuat **versi data user**. Setiap penulisan (transaksi, transfer, dompet, kategori, jadwal rutin, termasuk scheduler) menaikkan versi, jadi data lama tidak pernah tersaji |
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
| `finance/service/recurring.go` `runMu` | `sync.Mutex` | Dua sweep scheduler berjalan bersamaan |
| `finance/service/recurring.go` `RunDue` | worker pool goroutine + `sync.WaitGroup` + `atomic` | Memproses banyak jadwal jatuh tempo secara paralel |
| `finance/service/recurring.go` `RunScheduler` | goroutine + `time.Ticker` | Mencatat transaksi rutin tiap jam dan mengejar ketertinggalan saat server mati |
| `usecase/audit.go` | channel + 2 worker goroutine | Lookup lokasi yang lambat tidak menunda login |
| `usecase/auth.go` `touchMu` | `sync.Mutex` + goroutine `Maintain` | Buffer "terakhir aktif" ditulis massal tiap 30 detik, bukan per request |
| `usecase/common.go` `parallel()` | goroutine + `WaitGroup` + `Mutex` (error pertama) | Query dashboard dan laporan berjalan bersamaan |
| `platform/ratelimit`, `platform/geoip` | `sync.Mutex` | Map bersama diakses banyak request sekaligus |
| `finance/service/transfer.go` | `keymutex.Lock(asal, tujuan)` (2–4 kunci saat edit) | Transfer A→B dan B→A bersamaan tidak deadlock dan tidak bisa membuat salah satu minus |
| `platform/cache` `TTL` | `sync.RWMutex` + goroutine janitor per cache | Banyak request membaca cache bersamaan, satu menulis; entri kedaluwarsa dibersihkan tiap menit |
| `platform/cache` `Versions` | `sync.Mutex` | Versi data per user dinaikkan dari request dan scheduler sekaligus |
| `repository/cached` epoch | `sync/atomic` | Invalidasi yang berbarengan dengan pembacaan DB tidak "hilang" |
| `usecase/notification.go` | channel + worker goroutine; ticker + pool 4 goroutine | Notifikasi anggaran muncul sesaat setelah belanja tanpa memperlambat request; penyapuan berkala untuk tagihan besok, pengingat malam, ringkasan Senin |
| `finance/service/goal.go` `perGoal` | `keymutex` per target | Dua penarikan bersamaan tidak membuat tabungan minus (diuji 20 paralel) |
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
frontend/src/
├─ main.tsx                Provider: Toast → Confirm → Auth → App (inti) → provider tiap modul → App
├─ App.tsx                 Route inti + halaman dari semua modul, guard role
├─ brand.ts                Nama produk, tagline, prefix localStorage (warna: token di styles.css)
├─ styles.css              DESIGN SYSTEM: token warna terang/gelap, komponen, layout responsif
├─ lib/                    api (header X-Org-Id), auth, app (membership, version, changed, readOnly, limitOf/hasFeature),
│                          org (daftar workspace + yang aktif), next (halaman tujuan setelah masuk),
│                          router, hooks (useFetch SWR), format, types (tipe inti)
├─ components/             AppShell (nav & tombol utama dari modul), Notifications (ikon/label dari server),
│                          OrgSwitcher (pindah workspace / buat tim),
│                          GoogleButton, ui, Icon
├─ pages/                  Inti: Landing, Auth, Home (bila tak ada modul), Security, Subscription,
│                          NotificationSettings, Team (anggota, undangan), Invite (terima undangan),
│                          AdminUsers, AdminAudit, AdminPromos
└─ modules/
   ├─ types.ts             Kontrak AppModule
   ├─ index.ts             Daftar modul yang dipasang
   └─ finance/             index.tsx (definisi modul), context.tsx (dompet, kategori, modal Catat),
                           types.ts, format.ts, limits.ts, components/, pages/
```

**Interaksi data:** halaman mengambil data lewat `useFetch('/…?v=' + version)`. Setiap aksi tulis
memanggil `changed()` (inti; modul keuangan membungkusnya agar dompet/kategori ikut dimuat ulang),
yang menaikkan `version` dan memuat ulang paket/pemakaian. Semua halaman yang terbuka lalu memuat
ulang, jadi angka di mana pun selalu konsisten tanpa state management tambahan.

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
