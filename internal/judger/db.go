package judger

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"gorm.io/gorm"
)

// LoadFromDB rebuilds the in-memory contest/problem state from the database.
func LoadFromDB(db *gorm.DB) (map[string]*Contest, map[string]*Problem, map[string]*Contest, error) {
	var dbContests []models.Contest
	if err := db.Find(&dbContests).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load contests: %w", err)
	}

	var dbProblems []models.Problem
	if err := db.Find(&dbProblems).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load problems: %w", err)
	}

	var dbAnnouncements []models.Announcement
	if err := db.Find(&dbAnnouncements).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load announcements: %w", err)
	}

	annByContest := make(map[string][]*Announcement)
	for _, a := range dbAnnouncements {
		annByContest[a.ContestID] = append(annByContest[a.ContestID], &Announcement{
			ID:          a.ID,
			Title:       a.Title,
			Description: a.Description,
			CreatedAt:   a.CreatedAt,
			UpdatedAt:   a.UpdatedAt,
		})
	}

	contests := make(map[string]*Contest, len(dbContests))
	for _, c := range dbContests {
		anns := annByContest[c.ID]
		sort.Slice(anns, func(i, j int) bool { return anns[i].CreatedAt.After(anns[j].CreatedAt) })
		contests[c.ID] = &Contest{
			ID:            c.ID,
			Name:          c.Name,
			StartTime:     c.StartTime,
			EndTime:       c.EndTime,
			ProblemIDs:    append([]string(nil), c.ProblemIDs...),
			Description:   c.Description,
			Announcements: anns,
		}
	}

	problems := make(map[string]*Problem, len(dbProblems))
	problemToContest := make(map[string]*Contest)
	for _, p := range dbProblems {
		prob, err := problemFromModel(p)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to parse problem %s: %w", p.ID, err)
		}
		problems[p.ID] = prob
		if parent, ok := contests[p.ContestID]; ok {
			problemToContest[p.ID] = parent
		}
	}

	return contests, problems, problemToContest, nil
}

func problemFromModel(p models.Problem) (*Problem, error) {
	prob := &Problem{
		ID:             p.ID,
		Name:           p.Name,
		Level:          p.Level,
		StartTime:      p.StartTime,
		EndTime:        p.EndTime,
		MaxSubmissions: p.MaxSubmissions,
		Cluster:        p.Cluster,
		Description:    p.Description,
	}
	if len(p.Upload) > 0 {
		if err := json.Unmarshal(p.Upload, &prob.Upload); err != nil {
			return nil, fmt.Errorf("parse upload: %w", err)
		}
	}
	if len(p.Workflow) > 0 {
		if err := json.Unmarshal(p.Workflow, &prob.Workflow); err != nil {
			return nil, fmt.Errorf("parse workflow: %w", err)
		}
		// Backfill derived fields that the disk loader set but the DB doesn't store.
		for i := range prob.Workflow {
			steps := prob.Workflow[i].Steps
			if len(steps) == 0 {
				prob.Workflow[i].Image = "busybox"
			}
		}
	}
	if len(p.Score) > 0 {
		if err := json.Unmarshal(p.Score, &prob.Score); err != nil {
			return nil, fmt.Errorf("parse score: %w", err)
		}
	}
	if prob.Score.Mode == "" {
		prob.Score.Mode = "score"
	}
	return prob, nil
}

// ProblemToModel converts a judger.Problem into a models.Problem for DB persistence.
func ProblemToModel(p *Problem, contestID string) (models.Problem, error) {
	upload, err := json.Marshal(p.Upload)
	if err != nil {
		return models.Problem{}, fmt.Errorf("marshal upload: %w", err)
	}
	workflow, err := json.Marshal(p.Workflow)
	if err != nil {
		return models.Problem{}, fmt.Errorf("marshal workflow: %w", err)
	}
	score, err := json.Marshal(p.Score)
	if err != nil {
		return models.Problem{}, fmt.Errorf("marshal score: %w", err)
	}
	return models.Problem{
		ID:             p.ID,
		ContestID:      contestID,
		Name:           p.Name,
		Level:          p.Level,
		StartTime:      p.StartTime,
		EndTime:        p.EndTime,
		MaxSubmissions: p.MaxSubmissions,
		Cluster:        p.Cluster,
		Upload:         models.RawJSON(upload),
		Workflow:       models.RawJSON(workflow),
		Score:          models.RawJSON(score),
		Description:    p.Description,
	}, nil
}

// ContestToModel converts a judger.Contest into a models.Contest for DB persistence.
func ContestToModel(c *Contest) models.Contest {
	return models.Contest{
		ID:          c.ID,
		Name:        c.Name,
		StartTime:   c.StartTime,
		EndTime:     c.EndTime,
		Description: c.Description,
		ProblemIDs:  models.StringArray(append([]string(nil), c.ProblemIDs...)),
	}
}
