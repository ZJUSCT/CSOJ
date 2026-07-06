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

// JSONMap is a helper type for storing JSON data in the database.
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
	AllocatedCores string  `json:"allocated_cores"` // e.g., "2,3,4"
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
	DockerID     string `gorm:"docker_id" json:"docker_id"`

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
	ID          string      `gorm:"primaryKey" json:"id"`
	Name        string      `json:"name"`
	StartTime   time.Time   `gorm:"index" json:"starttime"`
	EndTime     time.Time   `json:"endtime"`
	Description string      `gorm:"type:text" json:"description"`
	ProblemIDs  StringArray `gorm:"type:text" json:"problem_ids"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// Problem is a backend-managed problem definition.
type Problem struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	ContestID      string    `gorm:"index" json:"contest_id"`
	Name           string    `json:"name"`
	Level          string    `json:"level"`
	StartTime      time.Time `json:"starttime"`
	EndTime        time.Time `json:"endtime"`
	MaxSubmissions int       `json:"max_submissions"`
	Cluster        string    `gorm:"index" json:"cluster"`
	CPU            int       `json:"cpu"`
	Memory         int64     `json:"memory"`
	Upload         JSONMap   `gorm:"type:text" json:"upload"`
	Workflow       JSONMap   `gorm:"type:text" json:"workflow"`
	Score          JSONMap   `gorm:"type:text" json:"score"`
	Description    string    `gorm:"type:text" json:"description"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
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

// ClusterNode holds the runtime-mutable resource caps for a node.
// The Docker connection (host, TLS) lives in config.yaml and is merged at boot.
type ClusterNode struct {
	ClusterName string `gorm:"primaryKey" json:"cluster_name"`
	NodeName    string `gorm:"primaryKey" json:"node_name"`
	CPU         int    `json:"cpu"`
	Memory      int64  `json:"memory"`
}

// Link is a nav-bar link managed via the admin API.
type Link struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Position int    `json:"position"`
}
