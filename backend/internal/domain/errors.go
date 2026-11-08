package domain

import "errors"

// AppError is an expected failure with a stable code for clients and a message
// safe to show to the person using the app. The HTTP layer maps Kind to a status.
type AppError struct {
	Kind    ErrorKind
	Code    string
	Message string
	Field   string
}

func (e *AppError) Error() string { return e.Code + ": " + e.Message }

type ErrorKind int

const (
	KindInvalid ErrorKind = iota
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindTooMany
)

func newErr(kind ErrorKind, code, msg string) *AppError {
	return &AppError{Kind: kind, Code: code, Message: msg}
}

// Invalid reports bad input, optionally tied to a form field.
func Invalid(field, msg string) *AppError {
	return &AppError{Kind: KindInvalid, Code: "INVALID", Message: msg, Field: field}
}

func NotFound(msg string) *AppError { return newErr(KindNotFound, "NOT_FOUND", msg) }

func Conflict(code, msg string) *AppError { return newErr(KindConflict, code, msg) }

func Forbidden(code, msg string) *AppError { return newErr(KindForbidden, code, msg) }

func TooMany(msg string) *AppError { return newErr(KindTooMany, "TOO_MANY", msg) }

var (
	ErrUnauthenticated = newErr(KindUnauthorized, "UNAUTHENTICATED", "Sesi Anda sudah berakhir. Silakan masuk lagi.")
	ErrBadCredentials  = newErr(KindUnauthorized, "BAD_CREDENTIALS", "Email atau kata sandi salah.")
	ErrAdminOnly       = newErr(KindForbidden, "ADMIN_ONLY", "Halaman ini khusus admin.")
	ErrUserOnly        = newErr(KindForbidden, "USER_ONLY", "Fitur keuangan hanya untuk akun pengguna.")
	ErrAccountFrozen   = newErr(KindForbidden, "ACCOUNT_FROZEN", "Akun Anda sedang dibekukan. Data bisa dilihat, tetapi tidak bisa diubah.")
	ErrInsufficient    = newErr(KindConflict, "INSUFFICIENT_BALANCE", "Saldo dompet tidak cukup untuk transaksi ini.")
	// Google sign-in. Codes are passed to the sign-in page, which explains them.
	ErrOAuthDisabled   = newErr(KindNotFound, "OAUTH_DISABLED", "Masuk dengan Google belum diaktifkan di server ini.")
	ErrOAuthFailed     = newErr(KindUnauthorized, "OAUTH_FAILED", "Masuk dengan Google gagal. Coba lagi.")
	ErrOAuthUnverified = newErr(KindForbidden, "OAUTH_EMAIL_UNVERIFIED", "Email akun Google ini belum diverifikasi Google.")
	ErrOAuthAdmin      = newErr(KindForbidden, "OAUTH_ADMIN", "Akun admin hanya bisa masuk dengan email dan kata sandi.")
	// ErrNoRows lets repositories signal a missing row without leaking database/sql.
	ErrNoRows = errors.New("row not found")
)

// Suspended builds the login error that tells a suspended user why.
func Suspended(reason string) *AppError {
	msg := "Akun Anda ditangguhkan oleh admin."
	if reason != "" {
		msg += " Alasan: " + reason
	}
	return newErr(KindForbidden, "ACCOUNT_SUSPENDED", msg)
}
