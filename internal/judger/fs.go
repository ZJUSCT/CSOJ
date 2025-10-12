package judger

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// writeYamlFile marshals the data and writes it to the specified path.
func writeYamlFile(path string, data interface{}) error {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal yaml: %w", err)
	}
	return os.WriteFile(path, bytes, 0644)
}

// CreateContest creates a new contest directory and its configuration files.
// baseDir is the root directory where all contests are stored (e.g., "contests/").
func CreateContest(baseDir string, contest *Contest) error {
	contest.BasePath = filepath.Join(baseDir, contest.ID)
	if err := os.MkdirAll(contest.BasePath, 0755); err != nil {
		return fmt.Errorf("failed to create contest directory: %w", err)
	}

	// Create contest.yaml
	contestYamlPath := filepath.Join(contest.BasePath, "contest.yaml")
	if err := writeYamlFile(contestYamlPath, contest); err != nil {
		return err
	}

	// Create index.md
	contestMdPath := filepath.Join(contest.BasePath, "index.md")
	return os.WriteFile(contestMdPath, []byte(contest.Description), 0644)
}

// UpdateContest updates an existing contest's configuration files.
func UpdateContest(contest *Contest) error {
	if contest.BasePath == "" {
		return fmt.Errorf("contest base path is empty, cannot update")
	}

	// Update contest.yaml
	contestYamlPath := filepath.Join(contest.BasePath, "contest.yaml")
	if err := writeYamlFile(contestYamlPath, contest); err != nil {
		return err
	}

	// Update index.md
	contestMdPath := filepath.Join(contest.BasePath, "index.md")
	return os.WriteFile(contestMdPath, []byte(contest.Description), 0644)
}

// DeleteContest removes the entire directory for a contest.
func DeleteContest(contest *Contest) error {
	if contest.BasePath == "" {
		return fmt.Errorf("contest base path is empty, cannot delete")
	}
	return os.RemoveAll(contest.BasePath)
}

// CreateProblem creates a new problem directory and files within a contest.
// It also updates the parent contest's YAML file.
func CreateProblem(contest *Contest, problem *Problem) error {
	problem.BasePath = filepath.Join(contest.BasePath, problem.ID)
	if err := os.MkdirAll(problem.BasePath, 0755); err != nil {
		return fmt.Errorf("failed to create problem directory: %w", err)
	}

	// Create problem.yaml and index.md
	if err := UpdateProblem(problem); err != nil {
		return err
	}

	// Update parent contest.yaml to include this new problem
	contest.ProblemDirs = append(contest.ProblemDirs, problem.ID)
	slices.Sort(contest.ProblemDirs) // Keep it sorted
	contest.ProblemDirs = slices.Compact(contest.ProblemDirs)
	return UpdateContest(contest)
}

// UpdateProblem updates an existing problem's configuration files.
func UpdateProblem(problem *Problem) error {
	if problem.BasePath == "" {
		return fmt.Errorf("problem base path is empty, cannot update")
	}

	// Update problem.yaml
	problemYamlPath := filepath.Join(problem.BasePath, "problem.yaml")
	if err := writeYamlFile(problemYamlPath, problem); err != nil {
		return err
	}

	// Update index.md
	problemMdPath := filepath.Join(problem.BasePath, "index.md")
	return os.WriteFile(problemMdPath, []byte(problem.Description), 0644)
}

// DeleteProblem removes a problem's directory and updates the parent contest's YAML.
func DeleteProblem(contest *Contest, problemID string) error {
	problemPath := filepath.Join(contest.BasePath, problemID)
	if err := os.RemoveAll(problemPath); err != nil {
		return fmt.Errorf("failed to delete problem directory: %w", err)
	}

	// Remove the problem from the parent contest's list
	var newProblemDirs []string
	for _, pDir := range contest.ProblemDirs {
		if pDir != problemID {
			newProblemDirs = append(newProblemDirs, pDir)
		}
	}
	contest.ProblemDirs = newProblemDirs

	return UpdateContest(contest)
}
