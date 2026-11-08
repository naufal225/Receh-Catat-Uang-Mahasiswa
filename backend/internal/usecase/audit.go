package usecase

import (
	"context"
	"log"
	"sync"
	"time"

	"receh/internal/domain"
)

// AuditEntry is what callers report. The recorder fills in the location later.
type AuditEntry struct {
	UserID    *int64
	Email     string
	Event     domain.AuditEvent
	Client    domain.ClientInfo
	Detail    string
	SessionID int64 // Set on login, so the device list gets the location too.
}

type geoJob struct {
	auditID   int64
	sessionID int64
	ip        string
}

// AuditRecorder writes the audit row straight away, so an event is never lost,
// then resolves the location on background goroutines so a slow geolocation
// service cannot delay a login.
type AuditRecorder struct {
	audit    domain.AuditRepository
	sessions domain.SessionRepository
	geo      domain.GeoLocator

	jobs chan geoJob
	wg   sync.WaitGroup
	once sync.Once
}

func NewAuditRecorder(a domain.AuditRepository, s domain.SessionRepository, g domain.GeoLocator, workers int) *AuditRecorder {
	r := &AuditRecorder{audit: a, sessions: s, geo: g, jobs: make(chan geoJob, 256)}
	for range workers {
		r.wg.Add(1)
		go r.worker()
	}
	return r
}

func (r *AuditRecorder) worker() {
	defer r.wg.Done()
	for job := range r.jobs {
		r.resolve(job)
	}
}

func (r *AuditRecorder) resolve(job geoJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	place := r.geo.Locate(ctx, job.ip)
	if err := r.audit.SetLocation(ctx, job.auditID, place); err != nil {
		log.Printf("audit: set location: %v", err)
	}
	if job.sessionID > 0 {
		if err := r.sessions.SetLocation(ctx, job.sessionID, place); err != nil {
			log.Printf("audit: set session location: %v", err)
		}
	}
}

// Record stores the event and queues its location lookup. It uses its own
// context because an audit row must survive the client hanging up.
func (r *AuditRecorder) Record(e AuditEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := &domain.AuditLog{
		UserID: e.UserID, Email: e.Email, Event: e.Event,
		IP: e.Client.IP, Device: e.Client.Device, Detail: e.Detail,
	}
	if err := r.audit.Insert(ctx, row); err != nil {
		log.Printf("audit: insert %s: %v", e.Event, err)
		return
	}
	job := geoJob{auditID: row.ID, sessionID: e.SessionID, ip: e.Client.IP}
	select {
	case r.jobs <- job:
	default: // Queue full: skip the remote lookup but leave a truthful value.
		_ = r.audit.SetLocation(ctx, row.ID, "Tidak diketahui")
	}
}

// Close stops accepting jobs and waits for queued lookups to finish.
func (r *AuditRecorder) Close() {
	r.once.Do(func() { close(r.jobs) })
	r.wg.Wait()
}
