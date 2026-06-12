package subagent

import (
	"fmt"
	"sync"
	"time"
)

type AuditLog struct {
	ID        string
	Subagent  string
	Tool      string
	Action    string
	Decision  string
	Reason    string
	Timestamp time.Time
}

type AuditStore interface {
	Save(log AuditLog) error
}

type MemoryAuditStore struct {
	mu   sync.Mutex
	logs []AuditLog
}

func NewMemoryAuditStore() *MemoryAuditStore {
	return &MemoryAuditStore{logs: make([]AuditLog, 0)}
}

func (s *MemoryAuditStore) Save(log AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, log)
	return nil
}

func (s *MemoryAuditStore) List() []AuditLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuditLog, len(s.logs))
	copy(out, s.logs)
	return out
}

type AuditRecorder struct {
	store AuditStore
}

func NewAuditRecorder(store AuditStore) *AuditRecorder {
	if store == nil {
		store = NewMemoryAuditStore()
	}
	return &AuditRecorder{store: store}
}

func (r *AuditRecorder) RecordAudit(log AuditLog) error {
	if log.ID == "" {
		log.ID = fmt.Sprintf("audit-%d", time.Now().UnixNano())
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}
	if err := r.store.Save(log); err != nil {
		return fmt.Errorf("save audit: %w", err)
	}
	return nil
}
