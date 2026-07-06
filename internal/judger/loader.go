package judger

import (
	"os"
	"time"
)

type Announcement struct {
	ID          string    `yaml:"id" json:"id"`
	Title       string    `yaml:"title" json:"title"`
	CreatedAt   time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at" json:"updated_at"`
	Description string    `yaml:"description" json:"description"`
}

type Contest struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	StartTime     time.Time       `json:"starttime"`
	EndTime       time.Time       `json:"endtime"`
	ProblemIDs    []string        `json:"problem_ids"`
	Description   string          `json:"description"`
	Announcements []*Announcement `json:"announcements"`
}

type UploadLimit struct {
	MaxNum      int      `yaml:"maxnum" json:"max_num"`
	MaxSize     int      `yaml:"maxsize" json:"max_size"`
	UploadForm  bool     `yaml:"upload_form" json:"upload_form"`
	UploadFiles []string `yaml:"upload_files" json:"upload_files"`
	Editor      bool     `yaml:"editor" json:"editor"`
	EditorFiles []string `yaml:"editor_files" json:"editor_files"`
}

type TmpfsOptions struct {
	SizeBytes int64       `yaml:"size_bytes" json:"size_bytes,omitempty"`
	Mode      os.FileMode `yaml:"mode,omitempty" json:"mode,omitempty"`
	Options   [][]string  `yaml:"options,omitempty" json:"options,omitempty"`
}

type Mount struct {
	Type        string       `yaml:"type" json:"type"`
	Source      string       `yaml:"source" json:"source"`
	Target      string       `yaml:"target" json:"target"`
	ReadOnly    *bool        `yaml:"readonly" json:"readonly"`
	TmpfsOption TmpfsOptions `yaml:"tmpfs_options" json:"tmpfs_options,omitempty"`
}

type WorkflowStep struct {
	Name    string     `yaml:"name" json:"name"`
	Image   string     `yaml:"image" json:"image"`
	Root    bool       `yaml:"root" json:"root"`
	Timeout int        `yaml:"timeout" json:"timeout"`
	Show    bool       `yaml:"show" json:"show"`
	Steps   [][]string `yaml:"steps" json:"steps"`
	Mounts  []Mount    `yaml:"mounts" json:"mounts"`
	Network bool       `yaml:"network" json:"network"`
}

type ScoreConfig struct {
	Mode                string `yaml:"mode" json:"mode"`
	MaxPerformanceScore int    `yaml:"max_performance_score" json:"max_performance_score"`
}

type Problem struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Level          string         `json:"level"`
	StartTime      time.Time      `json:"starttime"`
	EndTime        time.Time      `json:"endtime"`
	MaxSubmissions int            `json:"max_submissions"`
	Cluster        string         `json:"cluster"`
	CPU            int            `json:"cpu"`
	Memory         int64          `json:"memory"`
	Upload         UploadLimit    `json:"upload"`
	Workflow       []WorkflowStep `json:"workflow"`
	Score          ScoreConfig    `json:"score"`
	Description    string         `json:"description"`
}
