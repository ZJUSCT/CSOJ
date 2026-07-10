package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

type Status string

const (
	StatusQueued  Status = "Queued"
	StatusRunning Status = "Running"
	StatusSuccess Status = "Success"
	StatusFailed  Status = "Failed"
)

type Role string

const (
	RoleUser       Role = "user"
	RoleAdmin      Role = "admin"
	RoleSuperAdmin Role = "superadmin"
)

// JSONMap is a helper type for storing JSON object data in the database.
type JSONMap map[string]interface{}

func (m JSONMap) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *JSONMap) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, &m)
}

// RawJSON stores arbitrary JSON (object, array, or scalar) as raw bytes.
// Used for problem fields like Workflow (an array) that JSONMap cannot hold.
type RawJSON []byte

func (r RawJSON) Value() (driver.Value, error) {
	if r == nil {
		return nil, nil
	}
	return []byte(r), nil
}

func (r *RawJSON) Scan(value interface{}) error {
	if value == nil {
		*r = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*r = append((*r)[:0], v...)
		return nil
	case string:
		*r = append((*r)[:0], v...)
		return nil
	}
	return errors.New("type assertion to []byte or string failed")
}

func (r RawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return []byte(r), nil
}

func (r *RawJSON) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*r = nil
		return nil
	}
	*r = append((*r)[:0], data...)
	return nil
}

// StringArray is a []string stored as a JSON text column.
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	b, err := json.Marshal(a)
	return string(b), err
}

func (a *StringArray) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, a)
	case string:
		return json.Unmarshal([]byte(v), a)
	}
	return nil
}

type User struct {
	ID        string `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	GitLabID     *string    `gorm:"uniqueIndex" json:"-"`
	Username     string     `gorm:"uniqueIndex" json:"username"`
	PasswordHash string     `json:"-"`
	Nickname     string     `json:"nickname"`
	Signature    string     `json:"signature"`
	AvatarURL    string     `json:"avatar_url"`
	BannedUntil  *time.Time `json:"banned_until"`
	BanReason    string     `json:"ban_reason"`
	DisableRank  bool       `gorm:"default:false" json:"disable_rank"`
	Tags         string     `gorm:"type:text" json:"tags"` // Comma-separated tags
	Role         Role       `gorm:"type:text;default:'user';index" json:"role"`
}

type Submission struct {
	ID        string `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time
	UpdatedAt time.Time

	ProblemID string `gorm:"index" json:"problem_id"`
	UserID    string `gorm:"index" json:"user_id"`
	User      User   `json:"user"`

	Status         Status  `gorm:"index" json:"status"`
	CurrentStep    int     `json:"current_step"` // index of the current workflow step
	Cluster        string  `json:"cluster"`
	Node           string  `json:"node"`
	Score          int     `json:"score"`
	Performance    float64 `json:"performance"`
	Info           JSONMap `gorm:"type:text" json:"info"`
	IsValid        bool    `json:"is_valid"`

	Containers []Container `gorm:"foreignKey:SubmissionID;constraint:OnDelete:CASCADE" json:"containers"`
}

type Container struct {
	ID        string `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time
	UpdatedAt time.Time

	SubmissionID string `gorm:"index" json:"submission_id"`
	UserID       string `gorm:"index" json:"user_id"`
	User         User   `gorm:"foreignKey:UserID" json:"user"`
	PodName      string `json:"pod_name"`

	Image       string    `json:"image"`
	Status      Status    `json:"status"`
	ExitCode    int       `json:"exit_code"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	LogFilePath string    `json:"log_file_path"`
}

type ContestScoreHistory struct {
	ID                        uint `gorm:"primaryKey"`
	CreatedAt                 time.Time
	UserID                    string
	ContestID                 string
	ProblemID                 string
	TotalScoreAfterChange     int
	LastEffectiveSubmissionID string
}

type UserProblemBestScore struct {
	ID              uint   `gorm:"primaryKey"`
	UserID          string `gorm:"uniqueIndex:idx_user_problem"`
	ContestID       string `gorm:"uniqueIndex:idx_user_problem"`
	ProblemID       string `gorm:"uniqueIndex:idx_user_problem"`
	Score           int
	Performance     float64
	SubmissionID    string
	SubmissionCount int
	LastScoreTime   time.Time
}

// Contest is a backend-managed contest definition.
type Contest struct {
	ID                  string         `gorm:"primaryKey" json:"id"`
	Name                string         `json:"name"`
	StartTime           time.Time      `gorm:"index" json:"starttime"`
	EndTime             time.Time      `json:"endtime"`
	SubmitStartTime     *time.Time     `json:"submit_start_time,omitempty"`
	SubmitEndTime       *time.Time     `json:"submit_end_time,omitempty"`
	Description         string         `gorm:"type:text" json:"description"`
	ProblemIDs          StringArray    `gorm:"type:text" json:"problem_ids"`
	RegistrationConfig  RawJSON        `gorm:"type:text" json:"registration_config"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           gorm.DeletedAt `gorm:"index"`
}

// ContestRegistration tracks a user's registration for a contest.
type ContestRegistration struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	ContestID  string     `gorm:"index:idx_reg_user_contest,unique" json:"contest_id"`
	UserID     string     `gorm:"index:idx_reg_user_contest,unique" json:"user_id"`
	User       User       `gorm:"foreignKey:UserID" json:"user"`
	Status     string     `gorm:"default:pending" json:"status"` // "approved" | "pending" | "rejected"
	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	ReviewerID string     `json:"reviewer_id,omitempty"`
}

// Problem is a backend-managed problem definition.
type Problem struct {
	ID                string    `gorm:"primaryKey" json:"id"`
	ContestID         string    `gorm:"index" json:"contest_id"`
	Name              string    `json:"name"`
	Level             string    `json:"level"`
	StartTime         time.Time `json:"starttime"`
	EndTime           time.Time `json:"endtime"`
	SubmitStartTime   *time.Time `json:"submit_start_time,omitempty"`
	SubmitEndTime     *time.Time `json:"submit_end_time,omitempty"`
	MaxSubmissions    int       `json:"max_submissions"`
	Cluster           string    `gorm:"index" json:"cluster"`
	Upload            RawJSON   `gorm:"type:text" json:"upload"`
	Workflow          RawJSON   `gorm:"type:text" json:"workflow"`
	Score             RawJSON   `gorm:"type:text" json:"score"`
	DeadlineOverrides RawJSON   `gorm:"type:text" json:"deadline_overrides"`
	Description       string    `gorm:"type:text" json:"description"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Announcement is a contest-scoped announcement.
type Announcement struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	ContestID   string    `gorm:"index" json:"contest_id"`
	Title       string    `json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Asset is a contest/problem asset stored as a BLOB.
type Asset struct {
	ID        string    `gorm:"primaryKey" json:"-"`
	OwnerType string    `gorm:"index:idx_asset_owner" json:"-"` // "contest" | "problem"
	OwnerID   string    `gorm:"index:idx_asset_owner" json:"-"`
	Path      string    `gorm:"index:idx_asset_owner" json:"path"` // relative path, forward slashes
	IsDir     bool      `json:"is_dir"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	Content   []byte    `gorm:"type:blob" json:"-"`
}

// ClusterNodePool holds the runtime-mutable caps + nodeSelector for a pool.
// The K8s connection is cluster-level (config.yaml kubeconfig).
type ClusterNodePool struct {
	ClusterName  string    `gorm:"primaryKey" json:"cluster_name"`
	PoolName     string    `gorm:"primaryKey" json:"pool_name"`
	NodeSelector JSONMap   `gorm:"type:text" json:"node_selector"`
	CPU          int       `json:"cpu"`
	Memory       int64     `json:"memory"`
	IsPaused     bool      `gorm:"default:false" json:"is_paused"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Heartbeat tracks the live judger instance owning a cluster (HA).
type Heartbeat struct {
	ClusterName string    `gorm:"primaryKey" json:"cluster_name"`
	InstanceID  string    `json:"instance_id"`
	LastBeatAt  time.Time `json:"last_beat_at"`
}

// Link is a nav-bar link managed via the admin API.
type Link struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Position int    `json:"position"`
}

// Setting is a generic key/value runtime setting (JSON-encoded value).
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Cluster is a backend-managed K8s cluster (kubeconfig stored as text).
// Node-pool caps live in ClusterNodePool (cluster_name is the parent key).
type Cluster struct {
	Name         string    `gorm:"primaryKey" json:"name"`
	Kubeconfig   string    `gorm:"type:text" json:"kubeconfig"`
	Context      string    `json:"context"`
	Namespace    string    `json:"namespace"`
	Concurrency  int       `json:"concurrency"`
	HeartbeatTTL int       `json:"heartbeat_ttl"`
	QueueMode    string    `gorm:"default:channel" json:"queue_mode"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DevPodTemplate is an admin-defined fixed recipe for a user DevPod.
// The ID is kept short ([a-z0-9-]{1,6}) so the derived DevPod name
// <username>-<id>-<rand4> fits devpods' 22-char name budget.
type DevPodTemplate struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	Name            string    `json:"name"`
	ClusterName     string    `json:"cluster_name"`
	Image           string    `json:"image"`
	Shell           string    `json:"shell"`
	Cores           int       `json:"cores"`
	Memory          int64     `json:"memory"` // bytes
	NodeSelector    JSONMap   `gorm:"type:text" json:"node_selector"`
	Tolerations     RawJSON   `gorm:"type:text" json:"tolerations"`
	DefaultPerUser  int       `gorm:"default:1" json:"default_per_user"`
	DefaultGlobal   int       `gorm:"default:5" json:"default_global"`
	PersistenceSize string    `json:"persistence_size"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
