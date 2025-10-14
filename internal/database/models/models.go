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

	DockerID    string    `json:"-"`
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
