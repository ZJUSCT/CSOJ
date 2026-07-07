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
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	StartTime           time.Time          `json:"starttime"`
	EndTime             time.Time          `json:"endtime"`
	SubmitStartTime     *time.Time         `json:"submit_start_time,omitempty"`
	SubmitEndTime       *time.Time         `json:"submit_end_time,omitempty"`
	ProblemIDs          []string           `json:"problem_ids"`
	Description         string             `json:"description"`
	Announcements       []*Announcement    `json:"announcements"`
	RegistrationConfig  *RegistrationConfig `json:"registration_config,omitempty"`
}

// RegistrationConfig controls how users register for a contest.
type RegistrationConfig struct {
	Mode        string   `json:"mode"`                    // "auto" | "tag_auto" | "tag_review" | "review"
	AllowedTags []string `json:"allowed_tags,omitempty"`
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
	Name      string     `yaml:"name" json:"name"`
	Image     string     `yaml:"image" json:"image"`
	Root      bool       `yaml:"root" json:"root"`
	Timeout   int        `yaml:"timeout" json:"timeout"`
	Show      bool       `yaml:"show" json:"show"`
	Steps     [][]string `yaml:"steps" json:"steps"`
	Mounts    []Mount    `yaml:"mounts" json:"mounts"`
	Network   bool       `yaml:"network" json:"network"`
	MPI       *MPIConfig `yaml:"mpi,omitempty" json:"mpi,omitempty"`
	Resources  *StepResources  `yaml:"resources,omitempty" json:"resources,omitempty"`
	Scheduling *StepScheduling `yaml:"scheduling,omitempty" json:"scheduling,omitempty"`
}

// MPIConfig marks a workflow step as a multi-node MPI job.
// When non-nil and Enabled, the step runs as an MPIJob (mpi-operator).
type MPIConfig struct {
	Enabled        bool     `json:"enabled"`
	WorkerReplicas int      `json:"worker_replicas"`
	SlotsPerWorker int      `json:"slots_per_worker"`
	LauncherCmd    []string `json:"launcher_cmd"`
}

// StepResources holds the per-step CPU/memory requests and limits as K8s
// resource strings (e.g. "2", "500m", "1Gi"). Empty fields fall back to
// podspec defaults inside BuildPodSpec.
type StepResources struct {
	CPURequest    string `json:"cpu_request,omitempty"`
	CPULimit      string `json:"cpu_limit,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
	MemoryLimit   string `json:"memory_limit,omitempty"`
}

// StepScheduling carries the K8s scheduling constraints applied to a step's
// Pod (nodeSelector, nodeAffinity, tolerations, priorityClassName,
// runtimeClassName). All fields optional.
type StepScheduling struct {
	NodeSelector      map[string]string  `json:"node_selector,omitempty"`
	NodeAffinity      []NodeAffinityTerm `json:"node_affinity,omitempty"`
	Tolerations       []Toleration       `json:"tolerations,omitempty"`
	PriorityClassName string             `json:"priority_class_name,omitempty"`
	RuntimeClassName  string             `json:"runtime_class_name,omitempty"`
}

type NodeAffinityTerm struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

type Toleration struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"`
	Effect   string `json:"effect,omitempty"`
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
	SubmitStartTime *time.Time    `json:"submit_start_time,omitempty"`
	SubmitEndTime   *time.Time    `json:"submit_end_time,omitempty"`
	MaxSubmissions int            `json:"max_submissions"`
	Cluster        string         `json:"cluster"`
	Upload         UploadLimit    `json:"upload"`
	Workflow       []WorkflowStep `json:"workflow"`
	Score          ScoreConfig    `json:"score"`
	Description    string         `json:"description"`
}
