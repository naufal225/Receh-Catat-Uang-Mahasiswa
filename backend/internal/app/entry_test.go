package app_test

import (
	"fmt"
	"strings"
	"testing"

	"receh/internal/domain"
	"receh/internal/usecase"
)

func TestQuickEntryUsesTextThenHistory(t *testing.T) { // today 2026-01-15
	e := newEnv(t)
	c := e.newUser("")
	bca := c.wallet("BCA", 1_000_000)
	gopay := c.wallet("GoPay", 100_000)
	makan := c.category(domain.TxExpense, "Makan")

	d := must[domain.QuickDraft](c, 200, "POST", "/api/quick/parse", M{"text": "makan siang 25rb gopay kemarin"})
	if d.Amount != 25_000 || d.WalletID != gopay.ID || d.CategoryID != makan || d.OccurredOn != "2026-01-14" || d.Name != "Makan siang" || d.Type != domain.TxExpense {
		t.Fatalf("draft %+v", d)
	}
	// A name used before brings back its wallet and category.
	c.expect(201, "POST", "/api/transactions", M{"walletId": bca.ID, "categoryId": c.category(domain.TxExpense, "Kuliah"),
		"type": "expense", "name": "Fotokopi modul", "amount": 7_000, "occurredOn": "2026-01-10"})
	d = must[domain.QuickDraft](c, 200, "POST", "/api/quick/parse", M{"text": "fotokopi modul 8rb"})
	if d.WalletID != bca.ID || d.CategoryID != c.category(domain.TxExpense, "Kuliah") || d.Amount != 8_000 {
		t.Fatalf("history draft %+v", d)
	}
	c.expect(400, "POST", "/api/quick/parse", M{"text": "  "})

	// Suggestions while typing a name.
	c.expect(201, "POST", "/api/transactions", M{"walletId": bca.ID, "categoryId": c.category(domain.TxExpense, "Kuliah"),
		"type": "expense", "name": "Fotokopi modul", "amount": 9_000, "occurredOn": "2026-01-12"})
	s := must[[]domain.TxSuggestion](c, 200, "GET", "/api/transactions/suggest?q=foto", nil)
	if len(s) != 1 || s[0].Name != "Fotokopi modul" || s[0].Uses != 2 || s[0].Amount != 9_000 || s[0].WalletID != bca.ID {
		t.Fatalf("suggest %+v", s)
	}
	if s := must[[]domain.TxSuggestion](c, 200, "GET", "/api/transactions/suggest?q=f", nil); len(s) != 0 {
		t.Fatalf("one letter should not suggest: %+v", s)
	}
	if s := must[[]domain.TxSuggestion](e.newUser(""), 200, "GET", "/api/transactions/suggest?q=foto", nil); len(s) != 0 {
		t.Fatal("suggestions leaked to another user")
	}
}

const bcaCSV = "Tanggal Transaksi,Keterangan,Cabang,Jumlah,Saldo\n" +
	"'10/01,BAYAR NETFLIX,0000,\"54,000.00 DB\",\"0\"\n" +
	"'11/01,KR OTOMATIS UANG SAKU,0000,\"1,500,000.00 CR\",\"0\"\n" +
	"'12/01,ALFAMART,0000,\"38,500.00 DB\",\"0\"\n" +
	"'13/01,Fotokopi modul,0000,\"7,000.00 DB\",\"0\"\n"

func TestStatementImportPreviewCommitUndo(t *testing.T) {
	e := newEnv(t) // personal edition: import allowed
	c := e.newUser("")
	bca := c.wallet("BCA", 200_000)
	kuliah := c.category(domain.TxExpense, "Kuliah")
	// Already recorded by hand: the import must flag it as a duplicate.
	c.expect(201, "POST", "/api/transactions", M{"walletId": bca.ID, "categoryId": kuliah, "type": "expense",
		"name": "Fotokopi modul", "amount": 7_000, "occurredOn": "2026-01-13"})

	p := must[usecase.ImportPreview](c, 200, "POST", "/api/import/preview", M{"walletId": bca.ID, "csv": bcaCSV})
	if len(p.Rows) != 4 || p.Mapping.Amount != 3 {
		t.Fatalf("preview %+v", p)
	}
	byName := map[string]usecase.ImportRow{}
	for _, r := range p.Rows {
		byName[r.Name] = r
	}
	if r := byName["BAYAR NETFLIX"]; r.Type != domain.TxExpense || r.Amount != 54_000 || r.CategoryID != c.category(domain.TxExpense, "Langganan") {
		t.Fatalf("netflix row %+v", r)
	}
	if r := byName["KR OTOMATIS UANG SAKU"]; r.Type != domain.TxIncome || r.CategoryID != c.category(domain.TxIncome, "Uang Saku") {
		t.Fatalf("income row %+v", r)
	}
	if r := byName["ALFAMART"]; r.CategoryID != c.category(domain.TxExpense, "Lainnya") {
		t.Fatalf("fallback row %+v", r)
	}
	if r := byName["Fotokopi modul"]; !r.Duplicate || r.CategoryID != kuliah {
		t.Fatalf("duplicate row %+v", r)
	}

	// Import everything but the duplicate.
	var rows []usecase.ImportRow
	for _, r := range p.Rows {
		if !r.Duplicate {
			rows = append(rows, r)
		}
	}
	b := must[domain.ImportBatch](c, 201, "POST", "/api/import/commit", M{"walletId": bca.ID, "fileName": "mutasi-jan.csv", "rows": rows})
	if b.Rows != 3 || b.TotalIn != 1_500_000 || b.TotalOut != 92_500 {
		t.Fatalf("batch %+v", b)
	}
	// 200.000 - 7.000 (manual) + 1.500.000 - 92.500
	if got := c.balance(bca.ID); got != 1_600_500 {
		t.Fatalf("balance after import %d", got)
	}
	list := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions", nil)
	imported := 0
	for _, tx := range list.Items {
		if tx.ImportBatchID != nil && *tx.ImportBatchID == b.ID {
			imported++
		}
	}
	if imported != 3 {
		t.Fatalf("imported rows %d", imported)
	}
	batches := must[[]domain.ImportBatch](c, 200, "GET", "/api/import/batches", nil)
	if len(batches) != 1 || batches[0].Remaining != 3 || batches[0].FileName != "mutasi-jan.csv" {
		t.Fatalf("batches %+v", batches)
	}

	// Undo removes exactly the import and restores the balance.
	c.expect(204, "DELETE", fmt.Sprintf("/api/import/batches/%d", b.ID), nil)
	c.expect(404, "DELETE", fmt.Sprintf("/api/import/batches/%d", b.ID), nil)
	if got := c.balance(bca.ID); got != 193_000 {
		t.Fatalf("balance after undo %d", got)
	}
	if n := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions", nil).Total; n != 1 {
		t.Fatalf("rows after undo %d", n)
	}

	// Validation and money safety.
	bad := rows[0]
	bad.CategoryID = c.category(domain.TxIncome, "Beasiswa") // expense row, income category
	c.expect(400, "POST", "/api/import/commit", M{"walletId": bca.ID, "rows": []usecase.ImportRow{bad}})
	big := rows[0]
	big.Amount = 10_000_000
	body := c.expect(409, "POST", "/api/import/commit", M{"walletId": bca.ID, "rows": []usecase.ImportRow{big}})
	if !strings.Contains(body, "sesuaikan saldo") {
		t.Fatalf("insufficient message: %s", body)
	}
	// A file whose columns are not recognised returns no rows and no mapping, so
	// the page asks the person to point at the date, description and amount columns.
	if u := must[usecase.ImportPreview](c, 200, "POST", "/api/import/preview", M{"walletId": bca.ID, "csv": "A,B\nx,y\n"}); len(u.Rows) != 0 || u.Mapping.Date != -1 {
		t.Fatalf("unrecognised file %+v", u)
	}
	other := e.newUser("")
	other.expect(400, "POST", "/api/import/preview", M{"walletId": bca.ID, "csv": bcaCSV})
}

func TestStatementImportIsPremium(t *testing.T) {
	e := newB2CEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 100_000)
	body := c.expect(403, "POST", "/api/import/preview", M{"walletId": w.ID, "csv": bcaCSV})
	if !strings.Contains(body, "PLAN_LIMIT") {
		t.Fatalf("free plan import: %s", body)
	}
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	e.admin().expect(201, "POST", fmt.Sprintf("/api/admin/users/%d/premium", me.ID), M{"note": "Keluarga"})
	c.expect(200, "POST", "/api/import/preview", M{"walletId": w.ID, "csv": bcaCSV})
}
