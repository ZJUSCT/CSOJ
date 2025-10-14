package judger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type Announcement struct {
	ID          string    `yaml:"id" json:"id"`
	Title       string    `yaml:"title" json:"title"`
	CreatedAt   time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at" json:"updated_at"`
	Description string    `yaml:"description" json:"description"`
}

type Contest struct {
	ID            string          `yaml:"id" json:"id"`
	Name          string          `yaml:"name" json:"name"`
	StartTime     time.Time       `yaml:"starttime" json:"starttime"`
	EndTime       time.Time       `yaml:"endtime" json:"endtime"`
	ProblemDirs   []string        `yaml:"problems" json:"-"` // Renamed from ProblemDirs to problems in YAML, hide from JSON
	ProblemIDs    []string        `yaml:"-" json:"problem_ids"`
	Description   string          `yaml:"-" json:"description"`
	BasePath      string          `yaml:"-" json:"-"`             // Store the base path to find assets, hide from both
	Announcements []*Announcement `yaml:"-" json:"announcements"` // Loaded from announcements.yaml, hidden from contest.yaml
}

type UploadLimit struct {
	MaxNum      int      `yaml:"maxnum" json:"max_num"`
	MaxSize     int      `yaml:"maxsize" json:"max_size"`
	UploadForm  bool     `yaml:"upload_form" json:"upload_form"`
	UploadFiles []string `yaml:"upload_files" json:"upload_files"`
	Editor      bool     `yaml:"editor" json:"editor"`
	EditorFiles []string `yaml:"editor_files" json:"editor_files"`
}

type Mount struct {
	Type     string `yaml:"type" json:"type"`
	Source   string `yaml:"source" json:"source"`
	Target   string `yaml:"target" json:"target"`
	ReadOnly *bool  `yaml:"readonly" json:"readonly"`
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
	ID             string         `yaml:"id" json:"id"`
	Name           string         `yaml:"name" json:"name"`
	Level          string         `yaml:"level" json:"level"`
	StartTime      time.Time      `yaml:"starttime" json:"starttime"`
	EndTime        time.Time      `yaml:"endtime" json:"endtime"`
	MaxSubmissions int            `yaml:"max_submissions" json:"max_submissions"`
	Cluster        string         `yaml:"cluster" json:"cluster"`
	CPU            int            `yaml:"cpu" json:"cpu"`
	Memory         int64          `yaml:"memory" json:"memory"`
	Upload         UploadLimit    `yaml:"upload" json:"upload"`
	Workflow       []WorkflowStep `yaml:"workflow" json:"workflow"`
	Score          ScoreConfig    `yaml:"score" json:"score"`
	Description    string         `json:"description"`
	BasePath       string         `yaml:"-" json:"-"` // Store the base path to find assets, hide from both
}

// FindContestDirs scans a root directory and returns a slice of all its immediate subdirectories.
func FindContestDirs(rootPath string) ([]string, error) {
	if rootPath == "" {
		zap.S().Warn("contests_root is not configured. No contests will be loaded.")
		return []string{}, nil
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read contests_root directory '%s': %w", rootPath, err)
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(rootPath, entry.Name()))
		}
	}
	return dirs, nil
}

func LoadAllContestsAndProblems(contestDirs []string) (map[string]*Contest, map[string]*Problem, error) {
	contests := make(map[string]*Contest)
	problems := make(map[string]*Problem)

	for _, dir := range contestDirs {
		contest, contestProblems, err := loadContest(dir)
		if err != nil {
			zap.S().Warnf("failed to load contest from %s: %v", dir, err)
			continue
		}
		if _, exists := contests[contest.ID]; exists {
			zap.S().Warnf("duplicate contest ID %s found, skipping", dir)
			continue
		}
		contests[contest.ID] = contest

		for _, p := range contestProblems {
			if _, exists := problems[p.ID]; exists {
				zap.S().Warnf("duplicate problem ID %s found, overwriting", p.ID)
			}
			problems[p.ID] = p
		}
	}
	return contests, problems, nil
}

func loadContest(dir string) (*Contest, []*Problem, error) {
	// Load contest.yaml
	contestPath := filepath.Join(dir, "contest.yaml")
	data, err := os.ReadFile(contestPath)
	if err != nil {
		return nil, nil, err
	}
	var contest Contest
	if err := yaml.Unmarshal(data, &contest); err != nil {
		return nil, nil, err
	}
	contest.BasePath = dir // Set the base path

	// Load contest description
	desc, _ := os.ReadFile(filepath.Join(dir, "index.md"))
	contest.Description = string(desc)

	// Load announcements
	announcementsPath := filepath.Join(dir, "announcements.yaml")
	if annData, err := os.ReadFile(announcementsPath); err == nil {
		var announcements []*Announcement
		if err := yaml.Unmarshal(annData, &announcements); err == nil {
			// Sort announcements by CreatedAt descending (newest first)
			sort.Slice(announcements, func(i, j int) bool {
				return announcements[i].CreatedAt.After(announcements[j].CreatedAt)
			})
			contest.Announcements = announcements
		} else {
			zap.S().Warnf("failed to parse announcements.yaml for contest %s: %v", contest.ID, err)
		}
	}

	var loadedProblems []*Problem
	for _, problemDirName := range contest.ProblemDirs {
		problem, err := loadProblem(filepath.Join(dir, problemDirName))
		if err != nil {
			zap.S().Warnf("failed to load problem %s in contest %s: %v", problemDirName, contest.ID, err)
			continue
		}
		contest.ProblemIDs = append(contest.ProblemIDs, problem.ID)
		loadedProblems = append(loadedProblems, problem)
	}
	return &contest, loadedProblems, nil
}

func loadProblem(dir string) (*Problem, error) {
	problemPath := filepath.Join(dir, "problem.yaml")
	data, err := os.ReadFile(problemPath)
	if err != nil {
		return nil, err
	}
	var problem Problem
	if err := yaml.Unmarshal(data, &problem); err != nil {
		return nil, err
	}
	problem.BasePath = dir // Set the base path

	// Set default score mode if not provided
	if problem.Score.Mode == "" {
		problem.Score.Mode = "score"
	}

	desc, _ := os.ReadFile(filepath.Join(dir, "index.md"))
	problem.Description = string(desc)
	return &problem, nil
}
