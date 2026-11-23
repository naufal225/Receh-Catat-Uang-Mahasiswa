package app_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"receh/internal/app"
	"receh/internal/repository/mysql"
	"receh/internal/usecase"
)

// A database created before workspaces existed (schema fixtures in testdata)
// must come through the upgrade with every row intact, each owned by its
// user's personal workspace. This is the safety net for real data.
func TestUpgradeFromBeforeWorkspacesKeepsEveryRow(t *testing.T) {
	cfg, err := driver.ParseDSN(testServerDSN())
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Skipf("MySQL tidak terjangkau: %v", err)
	}
	name := fmt.Sprintf("receh_test_%d_%d", time.Now().UnixNano()%1_000_000_000, dbCounter.Add(1))
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	cfg.DBName = name

	// Old world: the previous schema plus rows, including an admin account that
	// an older version gave starter categories.
	raw, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"testdata/pre_workspace_core.sql", "testdata/pre_workspace_finance.sql"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := mysql.ApplySchema(raw, string(src)); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`INSERT INTO users (id,name,email,password_hash,role,status,created_at) VALUES
			(1,'Admin','a@x.test','h','admin','active',1),(2,'Naufal','n@x.test','h','user','active',1),(3,'Sari','s@x.test','h','user','active',1)`,
		`INSERT INTO wallets (id,user_id,name,type,color,balance,created_at) VALUES
			(1,2,'BCA','bank','c1',900000,1),(2,2,'GoPay','ewallet','c2',50000,1),(3,3,'BCA','bank','c1',70000,1)`,
		`INSERT INTO categories (id,user_id,name,type,color) VALUES
			(1,2,'Makan','expense','c1'),(2,2,'Gaji','income','c3'),(3,3,'Makan','expense','c1'),(4,1,'Sisa admin','expense','c1')`,
		`INSERT INTO transactions (id,user_id,wallet_id,category_id,type,name,amount,occurred_on,created_at) VALUES
			(1,2,1,2,'income','Gaji',1000000,'2026-01-01',1),(2,2,1,1,'expense','Makan',100000,'2026-01-02',1),(3,3,3,3,'expense','Bakso',5000,'2026-01-03',1)`,
		`INSERT INTO budgets (id,user_id,category_id,amount,created_at) VALUES (1,2,1,500000,1)`,
		`INSERT INTO goals (id,user_id,name,target_amount,color,created_at) VALUES (1,2,'Laptop',8000000,'c4',1)`,
		`INSERT INTO recurring (id,user_id,wallet_id,category_id,type,name,amount,frequency,anchor_day,next_due) VALUES (1,2,2,1,'expense','Kos',500000,'monthly',1,'2026-02-01')`,
		// A run that died halfway: wallets already lost their user foreign key.
		`ALTER TABLE wallets ADD COLUMN org_id BIGINT NULL`,
		`ALTER TABLE wallets DROP FOREIGN KEY fk_wallets_user`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	raw.Close()

	// Upgrade twice: the second start must be a no-op.
	for range 2 {
		db, err := mysql.Open(cfg.FormatDSN(), mysql.Options{})
		if err != nil {
			t.Fatal(err)
		}
		a, err := app.New(app.Params{DB: db, Clock: usecase.RealClock{Loc: time.UTC}, BcryptCost: 4})
		if err != nil {
			t.Fatal(err)
		}
		a.Close()
		db.Close()
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	count := func(q string) (n int64) {
		t.Helper()
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return
	}
	for table, want := range map[string]int64{"wallets": 3, "categories": 4, "transactions": 3, "budgets": 1, "goals": 1, "recurring": 1} {
		if got := count("SELECT COUNT(*) FROM " + table); got != want {
			t.Errorf("%s: %d rows, want %d", table, got, want)
		}
		if bad := count("SELECT COUNT(*) FROM " + table + " t JOIN organizations o ON o.id = t.org_id WHERE o.owner_id <> t.legacy_user_id"); bad != 0 {
			t.Errorf("%s: %d rows landed in someone else's workspace", table, bad)
		}
	}
	if got := count("SELECT SUM(balance) FROM wallets"); got != 1_020_000 {
		t.Errorf("balance sum %d", got)
	}
	// Three accounts, three personal workspaces (the admin got one for its leftovers).
	if got := count("SELECT COUNT(*) FROM organizations WHERE personal = 1"); got != 3 {
		t.Errorf("%d personal workspaces", got)
	}
	// Foreign keys now point at workspaces and the old constraint is gone.
	if got := count("SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE TABLE_SCHEMA = DATABASE() AND CONSTRAINT_NAME LIKE 'fk\\_%\\_org' AND TABLE_NAME IN ('wallets','categories','transactions','budgets','goals','recurring')"); got != 6 {
		t.Errorf("%d org foreign keys", got)
	}

}
