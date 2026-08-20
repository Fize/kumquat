package model

import "time"

// ResourceRecord is the API-owned desired-state record. EngineName and
// EngineNamespace identify its execution projection; ParentID is immutable.
type ResourceRecord struct {
	ID   string `gorm:"primaryKey;size:40" json:"id"`
	Kind string `gorm:"size:32;not null;index" json:"kind"`
	Name string `gorm:"size:253;not null" json:"name"`
	// ActiveKey is the Engine-scoped identity while the record is active. It is
	// cleared on archive so audit rows do not prevent legitimate recreation.
	ActiveKey       *string    `gorm:"size:600;uniqueIndex" json:"-"`
	ParentID        string     `gorm:"size:40;index" json:"parentId,omitempty"`
	ModuleID        *uint      `gorm:"index" json:"moduleId,omitempty"`
	ProjectID       *uint      `gorm:"index" json:"projectId,omitempty"`
	ProjectPublicID string     `gorm:"size:40;index" json:"-"`
	ModulePublicID  string     `gorm:"size:40;index" json:"-"`
	EngineName      string     `gorm:"size:253;not null" json:"engineName"`
	EngineNamespace string     `gorm:"size:253" json:"engineNamespace,omitempty"`
	DesiredJSON     string     `gorm:"type:text" json:"-"`
	DesiredHash     string     `gorm:"size:64" json:"-"`
	DesiredRevision uint64     `gorm:"not null;default:1" json:"-"`
	AppliedRevision uint64     `gorm:"not null;default:0" json:"-"`
	ObservedJSON    string     `gorm:"type:text" json:"-"`
	ObservedAt      *time.Time `json:"observedAt,omitempty"`
	SyncState       string     `gorm:"size:32;not null;default:pending" json:"syncState"`
	Source          string     `gorm:"size:32;not null;default:api" json:"source"`
	State           string     `gorm:"size:32" json:"state,omitempty"`
	CapabilityError string     `gorm:"type:text" json:"-"`
	ArchivedAt      *time.Time `gorm:"index" json:"archivedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type Operation struct {
	ID             string    `gorm:"primaryKey;size:40" json:"id"`
	IdempotencyKey string    `gorm:"size:128;uniqueIndex:idx_operation_actor_key" json:"-"`
	ActorID        uint      `gorm:"not null;uniqueIndex:idx_operation_actor_key;index" json:"-"`
	ModulePublicID string    `gorm:"size:40;index" json:"-"`
	Route          string    `gorm:"size:160;not null" json:"-"`
	Fingerprint    string    `gorm:"size:64;not null" json:"-"`
	ResourceID     string    `gorm:"size:40;index" json:"resourceId"`
	Action         string    `gorm:"size:32;not null" json:"action"`
	State          string    `gorm:"size:32;not null" json:"state"`
	Error          string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type OutboxEvent struct {
	ID              string     `gorm:"primaryKey;size:40"`
	OperationID     string     `gorm:"size:40;not null;uniqueIndex"`
	ResourceID      string     `gorm:"size:40;not null;index"`
	Action          string     `gorm:"size:32;not null"`
	DesiredRevision uint64     `gorm:"not null;default:0"`
	Attempts        int        `gorm:"not null;default:0"`
	LastError       string     `gorm:"type:text"`
	AvailableAt     time.Time  `gorm:"index"`
	ProcessedAt     *time.Time `gorm:"index"`
	ClaimToken      string     `gorm:"size:40;index"`
	ClaimUntil      *time.Time `gorm:"index"`
	CreatedAt       time.Time
}
