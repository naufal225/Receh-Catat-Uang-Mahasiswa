# Setup checklist (untuk pemilik produk)

Centang `[x]` saat sudah, dan tulis catatan di kolom **Status/Catatan**. File ini aman
di-commit: **jangan pernah menulis kunci/sandi asli di sini**. Nilai rahasia hanya
masuk ke `backend/.env` (sudah di-`.gitignore`).

Daftar ini dilengkapi tiap fase. Bagian bertanda *(Fase D/F/G)* baru dikerjakan nanti.

## 1. Keputusan produk

- [x] Paket: Gratis + Pro bulanan Rp19.000 + Pro tahunan Rp190.000 (diatur di `backend/internal/config/catalog.go`)
- [x] Hanya owner workspace yang boleh membayar
- [x] Masa tenggang 7 hari, tanpa trial
- [ ] Batas paket Gratis/Pro sudah final? (dompet 3 / anggaran 3 / target 1 / impor CSV; diatur di modul `finance`)  Status/Catatan: ______
- [ ] Kebijakan refund (tulis di Syarat Layanan; refund dilakukan manual lewat dashboard penyedia)  Status/Catatan: ______

## 2. Midtrans (sandbox dulu)

| Butir | Dari mana | Variabel `.env` | Status/Catatan |
|---|---|---|---|
| [ ] Akun sandbox | https://dashboard.sandbox.midtrans.com | - | |
| [ ] Server Key sandbox | Settings > Access Keys | `MIDTRANS_SERVER_KEY` | |
| [ ] `MIDTRANS_ENV=sandbox` | - | `MIDTRANS_ENV` | |
| [ ] URL notifikasi di dashboard | Settings > Payment > Notification URL | isi dengan `<PUBLIC_BASE_URL>/api/billing/webhook/midtrans` (halaman admin **Pembayaran** menampilkannya) | |
| [ ] `BILLING_PROVIDER=midtrans` | - | `BILLING_PROVIDER` | |
| [ ] Uji bayar sandbox (kartu/VA uji) berhasil sampai paket aktif | simulator Midtrans | - | |
| [ ] *(go-live)* Verifikasi bisnis + kunci production | dashboard production | `MIDTRANS_ENV=production` | |

## 3. Xendit (mode test dulu)

| Butir | Dari mana | Variabel `.env` | Status/Catatan |
|---|---|---|---|
| [ ] Akun, mode **Test** | https://dashboard.xendit.co | - | |
| [ ] Secret API key (test) | Settings > Developers > API Keys | `XENDIT_SECRET_KEY` | |
| [ ] Callback verification token | Settings > Developers > Webhooks | `XENDIT_CALLBACK_TOKEN` | |
| [ ] URL callback invoice | di halaman Webhooks, event "Invoices paid" (+ expired) | `<PUBLIC_BASE_URL>/api/billing/webhook/xendit` | |
| [ ] `BILLING_PROVIDER=xendit` | - | `BILLING_PROVIDER` | |
| [ ] **Cek nama field API Invoice di sandbox**: adaptor ditulis dari API v2 Invoice yang lama dan belum dicocokkan dengan dokumentasi terbaru (halamannya tak terbaca saat pembuatan). Jalankan satu alur bayar test dan pastikan paket aktif; bila gagal, lihat `backend/internal/platform/payment/xendit/xendit.go`. | - | - | |
| [ ] *(go-live)* Verifikasi bisnis + kunci live | dashboard | - | |

## 4. Webhook bisa dijangkau penyedia

- [ ] `PUBLIC_BASE_URL` diisi alamat publik (domain atau tunnel ngrok/cloudflared) di `backend/.env`
- [ ] Halaman admin **Pembayaran** menunjukkan semua kunci "terisi" (centang) dan menampilkan URL webhook
- [ ] Webhook uji dari dashboard penyedia dijawab 200

## 5. Menyusul

- [ ] *(Fase D)* SMTP: host, port, user, sandi, alamat pengirim; SPF/DKIM domain pengirim
- [ ] Google OAuth production: client id/secret, redirect URL domain asli, consent screen
- [ ] Domain + HTTPS (`COOKIE_SECURE=true`, `TRUST_PROXY=true` di belakang proxy)
- [ ] VPS + backup MySQL terjadwal (`mysqldump`) dan uji restore
- [ ] *(Fase F)* Legal: Kebijakan Privasi + Syarat Layanan (UU PDP), kontak pengendali data
- [ ] *(Fase G)* Kanal jual dan lisensi dipilih
