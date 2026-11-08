// Package usecase contains the application's behaviour. It depends only on the
// domain package and on ports, never on HTTP or SQL.
package usecase

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"receh/internal/domain"
)

// MaxAmount caps one amount at one trillion rupiah, far above any real entry and
// well inside int64, so sums cannot overflow.
const MaxAmount int64 = 1_000_000_000_000

// RealClock reports time in the application's configured timezone.
type RealClock struct{ Loc *time.Location }

func (c RealClock) Now() time.Time { return time.Now().In(c.Loc) }
func (c RealClock) Today() string  { return c.Now().Format(domain.DateLayout) }

// notifier tells caches that a user's data changed. Write methods call it with
// defer, even when they fail: a needless invalidation only costs one query,
// while a missed one would serve stale numbers.
type notifier struct{ n domain.ChangeNotifier }

func (x notifier) changed(userID int64) {
	if x.n != nil {
		x.n.Changed(userID)
	}
}

// parallel runs each function in its own goroutine, waits for all of them and
// returns the first error.
func parallel(fns ...func() error) error {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	for _, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func notFoundOr(err error, msg string) error {
	if errors.Is(err, domain.ErrNoRows) {
		return domain.NotFound(msg)
	}
	return err
}

func cleanName(field, value string, max int, emptyMsg string) (string, error) {
	v := strings.Join(strings.Fields(value), " ")
	if v == "" {
		return "", domain.Invalid(field, emptyMsg)
	}
	if utf8.RuneCountInString(v) > max {
		return "", domain.Invalid(field, "Maksimal "+itoa(max)+" karakter.")
	}
	return v, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func validAmount(field string, amount int64) error {
	if amount <= 0 {
		return domain.Invalid(field, "Nominal harus lebih dari nol.")
	}
	if amount > MaxAmount {
		return domain.Invalid(field, "Nominal terlalu besar.")
	}
	return nil
}

var colorRe = regexp.MustCompile(`^c[0-8]$`)

// validColor checks a palette slot token. Colours are stored as slots ("c1".."c8",
// "c0" neutral), not hex, so each theme can step its own validated values.
func validColor(c string) error {
	if !colorRe.MatchString(c) {
		return domain.Invalid("color", "Warna tidak valid.")
	}
	return nil
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func normalizeEmail(s string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(s))
	if len(e) > 254 || !emailRe.MatchString(e) {
		return "", domain.Invalid("email", "Format email belum benar.")
	}
	return e, nil
}

func validPassword(field, p string) error {
	if len(p) < 8 {
		return domain.Invalid(field, "Kata sandi minimal 8 karakter.")
	}
	if len(p) > 72 { // bcrypt only considers the first 72 bytes.
		return domain.Invalid(field, "Kata sandi maksimal 72 karakter.")
	}
	return nil
}

// DefaultCategories is what every new account starts with. Each type gets its
// slots in palette order, so a set of categories stays colourblind-safe.
func DefaultCategories() []domain.Category {
	return []domain.Category{
		{Name: "Makan", Type: domain.TxExpense, Color: "c1"},
		{Name: "Minum", Type: domain.TxExpense, Color: "c2"},
		{Name: "Transportasi", Type: domain.TxExpense, Color: "c3"},
		{Name: "Listrik", Type: domain.TxExpense, Color: "c4"},
		{Name: "Pulsa & Internet", Type: domain.TxExpense, Color: "c5"},
		{Name: "Kuliah", Type: domain.TxExpense, Color: "c6"},
		{Name: "Langganan", Type: domain.TxExpense, Color: "c7"},
		{Name: "Hiburan", Type: domain.TxExpense, Color: "c8"},
		{Name: FeeCategoryName, Type: domain.TxExpense, Color: "c0"},
		{Name: "Lainnya", Type: domain.TxExpense, Color: "c0"},
		{Name: "Uang Saku", Type: domain.TxIncome, Color: "c1"},
		{Name: "Beasiswa", Type: domain.TxIncome, Color: "c2"},
		{Name: "Kerja Sambilan", Type: domain.TxIncome, Color: "c3"},
		{Name: "Freelance", Type: domain.TxIncome, Color: "c4"},
		{Name: "Lainnya", Type: domain.TxIncome, Color: "c0"},
	}
}
