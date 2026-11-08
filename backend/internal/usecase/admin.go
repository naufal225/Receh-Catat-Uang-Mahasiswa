package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"receh/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type AdminService struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	logs     domain.AuditRepository
	audit    *AuditRecorder
	cost     int
}

func NewAdminService(u domain.UserRepository, s domain.SessionRepository, l domain.AuditRepository, a *AuditRecorder, cost int) *AdminService {
	return &AdminService{users: u, sessions: s, logs: l, audit: a, cost: cost}
}

// EnsureAdmin creates the first admin on an empty install. If no password was
// configured it generates one and returns it so the caller can show it once.
func (s *AdminService) EnsureAdmin(ctx context.Context, email, password string) (generated string, err error) {
	n, err := s.users.CountAdmins(ctx)
	if err != nil || n > 0 {
		return "", err
	}
	email, err = normalizeEmail(email)
	if err != nil {
		return "", err
	}
	if password == "" {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		password = base64.RawURLEncoding.EncodeToString(b)
		generated = password
	}
	if err := validPassword("password", password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", err
	}
	u := &domain.User{Name: "Administrator", Email: email, PasswordHash: string(hash), Role: domain.RoleAdmin, Status: domain.StatusActive}
	return generated, s.users.Create(ctx, u)
}

func (s *AdminService) ListUsers(ctx context.Context, f domain.UserFilter) (domain.Page[domain.User], error) {
	return s.users.List(ctx, f)
}

func (s *AdminService) CreateUser(ctx context.Context, actor *domain.User, name, email, password string, client domain.ClientInfo) (*domain.User, error) {
	name, err := cleanName("name", name, 60, "Nama wajib diisi.")
	if err != nil {
		return nil, err
	}
	email, err = normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if err := validPassword("password", password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return nil, err
	}
	u := &domain.User{Name: name, Email: email, PasswordHash: string(hash), Role: domain.RoleUser, Status: domain.StatusActive}
	if err := s.users.CreateWithDefaults(ctx, u, DefaultCategories()); err != nil {
		var app *domain.AppError
		if errors.As(err, &app) && app.Code == "EMAIL_TAKEN" {
			return nil, &domain.AppError{Kind: app.Kind, Code: app.Code, Message: app.Message, Field: "email"}
		}
		return nil, err
	}
	s.audit.Record(AuditEntry{UserID: &actor.ID, Email: actor.Email, Event: domain.EventUserCreated, Client: client, Detail: "Membuat akun " + u.Email})
	return u, nil
}

// SetStatus suspends, freezes or reactivates a user. Admin accounts are out of
// scope, so nobody can lock themselves or the platform out by accident.
func (s *AdminService) SetStatus(ctx context.Context, actor *domain.User, id int64, status domain.UserStatus, reason string, client domain.ClientInfo) (*domain.User, error) {
	if !status.Valid() {
		return nil, domain.Invalid("status", "Status tidak dikenal.")
	}
	target, err := s.users.ByID(ctx, id)
	if err != nil {
		return nil, notFoundOr(err, "Pengguna tidak ditemukan.")
	}
	if target.Role == domain.RoleAdmin {
		return nil, domain.Forbidden("ADMIN_PROTECTED", "Akun admin tidak bisa ditangguhkan atau dibekukan.")
	}
	if status == domain.StatusActive {
		reason = ""
	} else {
		reason, err = cleanName("reason", reason, 200, "Tulis alasannya. Pengguna akan melihat alasan ini.")
		if err != nil {
			return nil, err
		}
	}
	if err := s.users.UpdateStatus(ctx, id, status, reason); err != nil {
		return nil, err
	}
	if status == domain.StatusSuspended {
		if err := s.sessions.RevokeAll(ctx, id); err != nil {
			return nil, err
		}
	}
	detail := target.Email + ": " + string(target.Status) + " → " + string(status)
	if reason != "" {
		detail += " (" + reason + ")"
	}
	s.audit.Record(AuditEntry{UserID: &actor.ID, Email: actor.Email, Event: domain.EventStatusChanged, Client: client, Detail: detail})
	target.Status, target.StatusReason = status, reason
	return target, nil
}

func (s *AdminService) AuditLogs(ctx context.Context, f domain.AuditFilter) (domain.Page[domain.AuditLog], error) {
	return s.logs.List(ctx, f)
}
