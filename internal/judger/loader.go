package judger

import (
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type Contest struct {
	ID          string    `yaml:"id" json:"id"`
	Name        string    `yaml:"name" json:"name"`
	StartTime   time.Time `yaml:"starttime" json:"starttime"`
	EndTime     time.Time `yaml:"endtime" json:"endtime"`
	ProblemIDs  []string  `yaml:"problems" json:"problem_ids"`
	Description string    `json:"description"`
	BasePath    string    `json:"-"` // Store the base path to find assets
}

type UploadLimit struct {
	MaxNum  int `yaml:"maxnum" json:"max_num"`
	MaxSize int `yaml:"maxsize" json:"max_size"`
}

type WorkflowStep struct {
	Image   string     `yaml:"image" json:"image"`
	Root    bool       `yaml:"root" json:"root"`
	Timeout int        `yaml:"timeout" json:"timeout"`
	Show    bool       `yaml:"show" json:"show"`
	Steps   [][]string `yaml:"steps" json:"steps"`
}

type Problem struct {
	ID          string         `yaml:"id" json:"id"`
	Name        string         `yaml:"name" json:"name"`
	StartTime   time.Time      `yaml:"starttime" json:"starttime"`
	EndTime     time.Time      `yaml:"endtime" json:"endtime"`
	Cluster     string         `yaml:"cluster" json:"cluster"`
	CPU         int            `yaml:"cpu" json:"cpu"`
	Memory      int64          `yaml:"memory" json:"memory"`
	Upload      UploadLimit    `yaml:"upload" json:"upload"`
	Workflow    []WorkflowStep `yaml:"workflow" json:"workflow"`
	Description string         `json:"description"`
	BasePath    string         `json:"-"` // Store the base path to find assets
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
			zap.S().Warnf("duplicate contest ID %s found, skipping", contest.ID)
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

	var loadedProblems []*Problem
	for _, problemDirName := range contest.ProblemIDs {
		problem, err := loadProblem(filepath.Join(dir, problemDirName))
		if err != nil {
			zap.S().Warnf("failed to load problem %s in contest %s: %v", problemDirName, contest.ID, err)
			continue
		}
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

	desc, _ := os.ReadFile(filepath.Join(dir, "index.md"))
	problem.Description = string(desc)
	return &problem, nil
}
