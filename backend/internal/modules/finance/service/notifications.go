package service

import (
	"context"
	"strconv"
	"time"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
)

// Notification kinds of the finance module.
const (
	NotifyBillDue       domain.NotificationKind = "bill_due"       // a recurring bill is due tomorrow
	NotifyBillFailed    domain.NotificationKind = "bill_failed"    // a bill could not post: balance too low
	NotifyBudgetWarning domain.NotificationKind = "budget_warning" // a budget passed 80%
	NotifyBudgetOver    domain.NotificationKind = "budget_over"    // a budget passed 100%
	NotifyLowBalance    domain.NotificationKind = "low_balance"    // a wallet fell below the threshold
	NotifyGoalAchieved  domain.NotificationKind = "goal_achieved"  // a savings goal reached its target
	NotifyDailyReminder domain.NotificationKind = "daily_reminder" // nothing recorded today (evening)
	NotifyWeeklySummary domain.NotificationKind = "weekly_summary" // last week in numbers (Monday)

	SettingLowBalance   = "finance.low_balance_threshold"
	SettingReminderHour = "finance.reminder_hour"
)

// Notifications is the finance module's notification source: the core asks
// it what to say about a user and takes care of storage, dedup and delivery.
type Notifications struct {
	wallets   model.WalletRepository
	recurring model.RecurringRepository
	txs       model.TransactionRepository
	budgets   *BudgetService
	goals     *GoalService
	clock     domain.Clock
	appName   string
}

func NewNotifications(w model.WalletRepository, r model.RecurringRepository, t model.TransactionRepository,
	b *BudgetService, g *GoalService, clock domain.Clock, appName string) *Notifications {
	return &Notifications{wallets: w, recurring: r, txs: t, budgets: b, goals: g, clock: clock, appName: appName}
}

func lbl(id, en string) map[string]string { return map[string]string{"id": id, "en": en} }

func (s *Notifications) NotificationKinds() []domain.NotificationKindDef {
	return []domain.NotificationKindDef{
		{Key: NotifyBudgetWarning, Icon: "pie", Tone: "warn", Label: lbl("Anggaran hampir habis", "Budget almost used up")},
		{Key: NotifyBudgetOver, Icon: "pie", Tone: "danger", Label: lbl("Anggaran terlewati", "Budget exceeded")},
		{Key: NotifyBillDue, Icon: "repeat", Tone: "neutral", Label: lbl("Tagihan rutin besok", "Recurring bill due tomorrow")},
		{Key: NotifyBillFailed, Icon: "alert", Tone: "danger", Label: lbl("Tagihan gagal karena saldo kurang", "Bill failed: balance too low")},
		{Key: NotifyLowBalance, Icon: "wallet", Tone: "warn", Label: lbl("Saldo dompet menipis", "Low wallet balance")},
		{Key: NotifyGoalAchieved, Icon: "target", Tone: "ok", Label: lbl("Target tabungan tercapai", "Savings goal reached")},
		{Key: NotifyDailyReminder, Icon: "clock", Tone: "neutral", Label: lbl("Pengingat malam bila belum mencatat", "Evening reminder if nothing was recorded")},
		{Key: NotifyWeeklySummary, Icon: "chart", Tone: "neutral", Label: lbl("Ringkasan mingguan (Senin)", "Weekly summary (Monday)")},
	}
}

func (s *Notifications) NotificationSettings() []domain.NotificationSetting {
	return []domain.NotificationSetting{
		{Key: SettingLowBalance, Kind: "amount", Default: 50_000, Min: 0, Max: kitMaxAmount,
			Label: lbl("Saldo dianggap menipis di bawah", "Balance counts as low below")},
		{Key: SettingReminderHour, Kind: "hour", Default: 20, Min: 0, Max: 23,
			Label: lbl("Pengingat malam mulai pukul", "Evening reminder from")},
	}
}

// Evaluate reports what the finance module has to say about one user.
// timeDriven adds reminders that depend on the clock rather than a change.
func (s *Notifications) Evaluate(ctx context.Context, userID int64, timeDriven bool, prefs domain.NotificationPrefs, emit func(domain.Notification)) error {
	add := func(kind domain.NotificationKind, ref, title, body, link string) {
		emit(domain.Notification{UserID: userID, Kind: kind, RefKey: ref, Title: title, Body: body, Link: link})
	}
	lowBalance := prefs.Setting(SettingLowBalance, 50_000)
	reminderHour := int(prefs.Setting(SettingReminderHour, 20))
	today := s.clock.Today()
	day, _ := domain.ParseDate(today)

	// Budgets: one notice per budget per month and level.
	if m, err := s.budgets.Month(ctx, userID, ""); err != nil {
		return err
	} else {
		for _, b := range m.Items {
			id := strconv.FormatInt(b.ID, 10)
			switch b.Level {
			case model.BudgetOver:
				add(NotifyBudgetOver, "budget_over:"+id+":"+m.Month, "Anggaran "+b.CategoryName+" terlewati",
					"Terpakai "+formatRp(b.Spent)+" dari "+formatRp(b.Amount)+" ("+itoa(b.Percent)+"%) bulan ini.", "/app/anggaran")
			case model.BudgetWarning:
				add(NotifyBudgetWarning, "budget_warning:"+id+":"+m.Month, "Anggaran "+b.CategoryName+" hampir habis",
					"Sisa "+formatRp(b.Remaining)+" ("+itoa(b.Percent)+"% terpakai), "+itoa(m.DaysLeft)+" hari lagi bulan ini.", "/app/anggaran")
			}
		}
	}

	// Wallets: at most one low-balance notice per wallet per day, and only for
	// wallets in use (a brand-new empty wallet is not news).
	wallets, err := s.wallets.List(ctx, userID, false)
	if err != nil {
		return err
	}
	balance := map[int64]model.Wallet{}
	for _, w := range wallets {
		balance[w.ID] = w
		if lowBalance > 0 && w.Balance < lowBalance {
			used, err := s.wallets.HasTransactions(ctx, w.ID)
			if err != nil {
				return err
			}
			if used {
				add(NotifyLowBalance, "low_balance:"+strconv.FormatInt(w.ID, 10)+":"+today, "Saldo "+w.Name+" menipis",
					"Tinggal "+formatRp(w.Balance)+", di bawah batas "+formatRp(lowBalance)+" yang kamu atur.", "/app/dompet")
			}
		}
	}

	// Goals reached: once per goal.
	goals, err := s.goals.List(ctx, userID, false)
	if err != nil {
		return err
	}
	for _, g := range goals {
		if g.Achieved {
			add(NotifyGoalAchieved, "goal_achieved:"+strconv.FormatInt(g.ID, 10), "Target "+g.Name+" tercapai!",
				"Kamu sudah mengumpulkan "+formatRp(g.Saved)+". Saatnya merayakan, atau arsipkan targetnya.", "/app/target")
		}
	}

	// Recurring bills: failed for lack of money (event), due tomorrow (time).
	rules, err := s.recurring.List(ctx, userID)
	if err != nil {
		return err
	}
	tomorrow := day.AddDate(0, 0, 1).Format(domain.DateLayout)
	for _, r := range rules {
		if !r.Active || r.Type != model.TxExpense {
			continue
		}
		id := strconv.FormatInt(r.ID, 10)
		if r.LastError != "" {
			add(NotifyBillFailed, "bill_failed:"+id+":"+r.NextDue, r.Name+" belum bisa dicatat",
				"Saldo "+r.WalletName+" tidak cukup untuk "+formatRp(r.Amount)+". "+s.appName+" mencoba lagi otomatis setelah saldo cukup.", "/app/rutin")
		}
		if timeDriven && r.NextDue == tomorrow {
			body := formatRp(r.Amount) + " dari " + r.WalletName + " akan tercatat otomatis besok."
			if w, ok := balance[r.WalletID]; ok && w.Balance < r.Amount {
				body += " Saldo " + w.Name + " sekarang " + formatRp(w.Balance) + ", belum cukup."
			}
			add(NotifyBillDue, "bill_due:"+id+":"+r.NextDue, "Tagihan besok: "+r.Name, body, "/app/rutin")
		}
	}

	if timeDriven && len(wallets) > 0 {
		now := s.clock.Now()
		// Evening nudge when nothing was recorded today.
		if now.Hour() >= reminderHour {
			days, err := s.txs.SumByDay(ctx, userID, 0, today, today)
			if err != nil {
				return err
			}
			if len(days) == 0 {
				add(NotifyDailyReminder, "daily:"+today, "Sudah mencatat hari ini?",
					"Belum ada pemasukan atau pengeluaran hari ini. Catat sekarang selagi masih ingat.", "/app")
			}
		}
		// Monday: last week in numbers, compared with the week before.
		if day.Weekday() == time.Monday {
			if n, ok, err := s.weekly(ctx, userID, day); err != nil {
				return err
			} else if ok {
				add(NotifyWeeklySummary, n.RefKey, n.Title, n.Body, n.Link)
			}
		}
	}

	return nil
}

func (s *Notifications) weekly(ctx context.Context, userID int64, monday time.Time) (domain.Notification, bool, error) {
	from, to := monday.AddDate(0, 0, -7), monday.AddDate(0, 0, -1)
	sum := func(a, b time.Time) (in, out int64, err error) {
		days, err := s.txs.SumByDay(ctx, userID, 0, a.Format(domain.DateLayout), b.Format(domain.DateLayout))
		for _, d := range days {
			in += d.Income
			out += d.Expense
		}
		return in, out, err
	}
	in, out, err := sum(from, to)
	if err != nil || in+out == 0 {
		return domain.Notification{}, false, err
	}
	_, prevOut, err := sum(from.AddDate(0, 0, -7), from.AddDate(0, 0, -1))
	if err != nil {
		return domain.Notification{}, false, err
	}
	body := "Masuk " + formatRp(in) + ", keluar " + formatRp(out) + "."
	cats, err := s.txs.SumByCategory(ctx, userID, 0, model.TxExpense, from.Format(domain.DateLayout), to.Format(domain.DateLayout))
	if err != nil {
		return domain.Notification{}, false, err
	}
	if len(cats) > 0 {
		body += " Terbesar: " + cats[0].Name + " " + formatRp(cats[0].Total) + "."
	}
	if prevOut > 0 && out != prevOut {
		pct := (out - prevOut) * 100 / prevOut
		if pct > 0 {
			body += " Pengeluaran naik " + itoa64(pct) + "% dari minggu sebelumnya."
		} else if pct < 0 {
			body += " Pengeluaran turun " + itoa64(-pct) + "% dari minggu sebelumnya."
		}
	}
	return domain.Notification{
		RefKey: "weekly:" + from.Format(domain.DateLayout), Title: "Ringkasan minggu lalu", Body: body, Link: "/app",
	}, true, nil
}
