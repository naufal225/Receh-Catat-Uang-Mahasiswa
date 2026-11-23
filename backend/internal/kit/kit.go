// Package kit holds small helpers shared by the core and by modules: input
// cleaning, money formatting, goroutine fan-out and plan-limit checks. It
// depends only on package domain.
package kit

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"receh/internal/domain"
)

// MaxAmount caps one amount at one trillion, far above any real entry and
// well inside int64, so sums cannot overflow.
const MaxAmount int64 = 1_000_000_000_000

// Notifier tells caches and background workers that a user's data changed.
// Write methods call Changed with defer, even when they fail: a needless
// check costs one query, a missed one serves stale numbers.
type Notifier struct{ N domain.ChangeNotifier }

func (x Notifier) Changed(userID int64) {
	if x.N != nil {
		x.N.Changed(userID)
	}
}

// Parallel runs each function in its own goroutine, waits for all of them and
// returns the first error.
func Parallel(fns ...func() error) error {
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

// NotFoundOr turns a missing row into a friendly 404.
func NotFoundOr(err error, msg string) error {
	if errors.Is(err, domain.ErrNoRows) {
		return domain.NotFound(msg)
	}
	return err
}

// CleanName collapses whitespace and enforces a length.
func CleanName(field, value string, max int, emptyMsg string) (string, error) {
	v := strings.Join(strings.Fields(value), " ")
	if v == "" {
		return "", domain.Invalid(field, emptyMsg)
	}
	if utf8.RuneCountInString(v) > max {
		return "", domain.Invalid(field, "Maksimal "+Itoa(max)+" karakter.")
	}
	return v, nil
}

func Itoa(n int) string { return strconv.Itoa(n) }

func Itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func ValidAmount(field string, amount int64) error {
	if amount <= 0 {
		return domain.Invalid(field, "Nominal harus lebih dari nol.")
	}
	if amount > MaxAmount {
		return domain.Invalid(field, "Nominal terlalu besar.")
	}
	return nil
}

var colorRe = regexp.MustCompile(`^c[0-8]$`)

// ValidColor checks a palette slot token ("c1".."c8", "c0" neutral). Colours
// are stored as slots, not hex, so each theme can use its own values.
func ValidColor(c string) error {
	if !colorRe.MatchString(c) {
		return domain.Invalid("color", "Warna tidak valid.")
	}
	return nil
}

// FormatRp writes 101000 as "Rp 101.000".
func FormatRp(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	digits := strconv.FormatInt(v, 10)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	if neg {
		return "-Rp " + b.String()
	}
	return "Rp " + b.String()
}

// ScopeLockKey gives each workspace a keymutex key that can never equal a row id
// (row ids are positive), for "count, then create" checks against a limit.
func ScopeLockKey(orgID int64) int64 { return -orgID }

// PlanLimits is what modules ask before creating something limited.
type PlanLimits interface {
	// Limit returns the plan's limit for a key (domain.Unlimited for none, 0/1
	// for an on/off feature).
	Limit(ctx context.Context, userID int64, key string) (int, error)
}

// CheckLimit refuses one more item when current already reaches the limit.
func CheckLimit(ctx context.Context, p PlanLimits, userID int64, key string, current int, msg string) error {
	limit, err := p.Limit(ctx, userID, key)
	if err != nil {
		return err
	}
	if !domain.Allows(limit, current) {
		return domain.PlanLimit(strings.ReplaceAll(msg, "{limit}", Itoa(limit)))
	}
	return nil
}

// RequireFeature refuses when an on/off feature is off on the user's plan.
func RequireFeature(ctx context.Context, p PlanLimits, userID int64, key, msg string) error {
	limit, err := p.Limit(ctx, userID, key)
	if err != nil {
		return err
	}
	if limit == 0 {
		return domain.PlanLimit(msg)
	}
	return nil
}
