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
		CPU:            p.CPU,
		Memory:         p.Memory,
		Description:    p.Description,
	}
	if p.Upload != nil {
		var u UploadLimit
		if err := unmarshalJSONMap(p.Upload, &u); err == nil {
			prob.Upload = u
		}
	}
	if p.Workflow != nil {
		var w []WorkflowStep
		if err := unmarshalJSONMap(p.Workflow, &w); err == nil {
			prob.Workflow = w
		}
	}
	if p.Score != nil {
		var s ScoreConfig
		if err := unmarshalJSONMap(p.Score, &s); err == nil {
			prob.Score = s
		}
	}
	if prob.Score.Mode == "" {
		prob.Score.Mode = "score"
	}
	return prob, nil
}

// ProblemToModel converts a judger.Problem into a models.Problem for DB persistence.
func ProblemToModel(p *Problem, contestID string) (models.Problem, error) {
	upload, err := toJSONMap(p.Upload)
	if err != nil {
		return models.Problem{}, err
	}
	workflow, err := toJSONMap(p.Workflow)
	if err != nil {
		return models.Problem{}, err
	}
	score, err := toJSONMap(p.Score)
	if err != nil {
		return models.Problem{}, err
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
		CPU:            p.CPU,
		Memory:         p.Memory,
		Upload:         upload,
		Workflow:       workflow,
		Score:          score,
		Description:    p.Description,
	}, nil
}

func toJSONMap(v interface{}) (models.JSONMap, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m models.JSONMap
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func unmarshalJSONMap(m models.JSONMap, dst interface{}) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
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
