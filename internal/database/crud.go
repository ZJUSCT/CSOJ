package database

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// User CRUD
func CreateUser(db *gorm.DB, user *models.User) error {
	return db.Create(user).Error
}

func GetUserByID(db *gorm.DB, id string) (*models.User, error) {
	var user models.User
	if err := db.Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByUsername(db *gorm.DB, username string) (*models.User, error) {
	var user models.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByGitLabID(db *gorm.DB, gitlabID string) (*models.User, error) {
	var user models.User
	if err := db.Where("git_lab_id = ?", gitlabID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func GetAllUsers(db *gorm.DB) ([]models.User, error) {
	var users []models.User
	if err := db.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func UpdateUser(db *gorm.DB, user *models.User) error {
	return db.Save(user).Error
}

func DeleteUser(db *gorm.DB, userID string) error {
	return db.Delete(&models.User{}, "id = ?", userID).Error
}

// Submission CRUD
func CreateSubmission(db *gorm.DB, sub *models.Submission) error {
	return db.Create(sub).Error
}

func GetSubmission(db *gorm.DB, id string) (*models.Submission, error) {
	var sub models.Submission
	if err := db.Preload("User").Preload("Containers").Where("id = ?", id).First(&sub).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func GetSubmissionsByUserID(db *gorm.DB, userID string) ([]models.Submission, error) {
	var subs []models.Submission
	if err := db.Preload("User").Where("user_id = ?", userID).Order("created_at desc").Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}

func GetAllSubmissions(db *gorm.DB) ([]models.Submission, error) {
	var subs []models.Submission
	if err := db.Preload("User").Order("created_at desc").Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}

func UpdateSubmission(db *gorm.DB, sub *models.Submission) error {
	return db.Save(sub).Error
}

func UpdateSubmissionValidity(db *gorm.DB, id string, isValid bool) error {
	return db.Model(&models.Submission{}).Where("id = ?", id).Update("is_valid", isValid).Error
}

// CountQueuedSubmissionsBefore counts the number of submissions in the queue for a specific cluster that were created before a given time.
func CountQueuedSubmissionsBefore(db *gorm.DB, cluster string, createdAt time.Time) (int64, error) {
	var count int64
	err := db.Model(&models.Submission{}).
		Where("status = ? AND cluster = ? AND created_at < ?", models.StatusQueued, cluster, createdAt).
		Count(&count).Error
	return count, err
}

// Container CRUD
func CreateContainer(db *gorm.DB, container *models.Container) error {
	return db.Create(container).Error
}

func GetContainer(db *gorm.DB, id string) (*models.Container, error) {
	var container models.Container
	if err := db.Where("id = ?", id).First(&container).Error; err != nil {
		return nil, err
	}
	return &container, nil
}

func UpdateContainer(db *gorm.DB, container *models.Container) error {
	return db.Save(container).Error
}

// Score & Leaderboard

type LeaderboardEntry struct {
	UserID        string         `json:"user_id"`
	Username      string         `json:"username"`
	Nickname      string         `json:"nickname"`
	AvatarURL     string         `json:"avatar_url"`
	TotalScore    int            `json:"total_score"`
	ProblemScores map[string]int `json:"problem_scores"`
	lastScoreTime time.Time
}

// UserScoreHistoryPoint represents a single point in a user's score history for a contest.
type UserScoreHistoryPoint struct {
	Time      time.Time `json:"time"`
	Score     int       `json:"score"`
	ProblemID string    `json:"problem_id"`
}

func GetLeaderboard(db *gorm.DB, contestID string) ([]LeaderboardEntry, error) {
	// Intermediate struct for scanning the raw query result
	type scoreRow struct {
		UserID        string
		Username      string
		Nickname      string
		AvatarURL     string
		ProblemID     string
		Score         int
		LastScoreTime time.Time
	}
	var rows []scoreRow
	err := db.Table("user_problem_best_scores").
		Select("users.id as user_id, users.username, users.nickname, users.avatar_url, user_problem_best_scores.problem_id, user_problem_best_scores.score, user_problem_best_scores.last_score_time").
		Joins("join users on users.id = user_problem_best_scores.user_id").
		Where("user_problem_best_scores.contest_id = ?", contestID).
		Scan(&rows).Error

	if err != nil {
		return nil, err
	}

	// Process rows into a map for easy aggregation
	resultsMap := make(map[string]*LeaderboardEntry)
	for _, row := range rows {
		// If user is not yet in the map, create a new entry
		if _, ok := resultsMap[row.UserID]; !ok {

			if row.AvatarURL != "" && !strings.HasPrefix(row.AvatarURL, "http") {
				row.AvatarURL = fmt.Sprintf("/api/v1/assets/avatars/%s", row.AvatarURL)
			}

			resultsMap[row.UserID] = &LeaderboardEntry{
				UserID:        row.UserID,
				Username:      row.Username,
				Nickname:      row.Nickname,
				AvatarURL:     row.AvatarURL,
				TotalScore:    0,
				ProblemScores: make(map[string]int),
			}
		}
		// Add problem score, update total score, and track the latest time
		entry := resultsMap[row.UserID]
		entry.ProblemScores[row.ProblemID] = row.Score
		entry.TotalScore += row.Score

		// For tie-breaking, the time is the latest time any of their scores were improved.
		if row.LastScoreTime.After(entry.lastScoreTime) {
			entry.lastScoreTime = row.LastScoreTime
		}
	}

	// Convert map to slice
	var results []LeaderboardEntry
	for _, entry := range resultsMap {
		results = append(results, *entry)
	}

	// Sort the final slice by total score descending, then by time ascending for ties
	sort.Slice(results, func(i, j int) bool {
		if results[i].TotalScore != results[j].TotalScore {
			return results[i].TotalScore > results[j].TotalScore
		}
		// If scores are equal, the one with the earlier time is better (ranks higher).
		if results[i].lastScoreTime.IsZero() {
			return false
		}
		if results[j].lastScoreTime.IsZero() {
			return true
		}
		return results[i].lastScoreTime.Before(results[j].lastScoreTime)
	})

	return results, nil
}

// GetScoreHistoriesForUsers retrieves the score change history for a given list of users in a specific contest.
func GetScoreHistoriesForUsers(db *gorm.DB, contestID string, userIDs []string) (map[string][]UserScoreHistoryPoint, error) {
	var results []models.ContestScoreHistory
	if err := db.Model(&models.ContestScoreHistory{}).
		Where("contest_id = ? AND user_id IN ?", contestID, userIDs).
		Order("created_at asc").
		Find(&results).Error; err != nil {
		return nil, err
	}

	historiesByUser := make(map[string][]UserScoreHistoryPoint)
	for _, r := range results {
		// Initialize the slice for a user if it doesn't exist
		if _, ok := historiesByUser[r.UserID]; !ok {
			historiesByUser[r.UserID] = make([]UserScoreHistoryPoint, 0)
		}
		historiesByUser[r.UserID] = append(historiesByUser[r.UserID], UserScoreHistoryPoint{
			Time:      r.CreatedAt,
			Score:     r.TotalScoreAfterChange,
			ProblemID: r.ProblemID,
		})
	}
	return historiesByUser, nil
}

// GetScoreHistoryForUser retrieves the score change history for a specific user in a specific contest.
func GetScoreHistoryForUser(db *gorm.DB, contestID string, userID string) ([]UserScoreHistoryPoint, error) {
	var results []models.ContestScoreHistory
	if err := db.Model(&models.ContestScoreHistory{}).
		Where("contest_id = ? AND user_id = ?", contestID, userID).
		Order("created_at asc").
		Find(&results).Error; err != nil {
		return nil, err
	}

	history := make([]UserScoreHistoryPoint, 0, len(results))
	for _, r := range results {
		history = append(history, UserScoreHistoryPoint{
			Time:      r.CreatedAt,
			Score:     r.TotalScoreAfterChange,
			ProblemID: r.ProblemID,
		})
	}
	return history, nil
}

func RegisterForContest(db *gorm.DB, userID, contestID string) error {
	var count int64
	db.Model(&models.ContestScoreHistory{}).Where("user_id = ? AND contest_id = ?", userID, contestID).Count(&count)
	if count > 0 {
		return errors.New("already registered")
	}

	history := models.ContestScoreHistory{
		UserID:                userID,
		ContestID:             contestID,
		TotalScoreAfterChange: 0,
	}
	return db.Create(&history).Error
}

func GetSubmissionCount(db *gorm.DB, userID, contestID, problemID string) (int, error) {
	var scoreRecord models.UserProblemBestScore
	err := db.Where("user_id = ? AND contest_id = ? AND problem_id = ?", userID, contestID, problemID).
		First(&scoreRecord).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return scoreRecord.SubmissionCount, nil
}

func IncrementSubmissionCount(db *gorm.DB, userID, contestID, problemID string) error {
	record := models.UserProblemBestScore{
		UserID:          userID,
		ContestID:       contestID,
		ProblemID:       problemID,
		SubmissionCount: 1,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "contest_id"}, {Name: "problem_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"submission_count": gorm.Expr("submission_count + 1"),
		}),
	}).Create(&record).Error
}

func UpdateScoresForNewSubmission(db *gorm.DB, sub *models.Submission, contestID string, newScore int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Get current best score for the problem
		var bestScore models.UserProblemBestScore
		err := tx.Where("user_id = ? AND contest_id = ? AND problem_id = ?", sub.UserID, contestID, sub.ProblemID).
			First(&bestScore).Error

		// If no record exists or the new score is higher
		if errors.Is(err, gorm.ErrRecordNotFound) || newScore > bestScore.Score {
			// Update or create the best score record
			bestScore.UserID = sub.UserID
			bestScore.ContestID = contestID
			bestScore.ProblemID = sub.ProblemID
			bestScore.Score = newScore
			bestScore.SubmissionID = sub.ID
			bestScore.LastScoreTime = sub.CreatedAt // Update time only on score increase
			if err := tx.Save(&bestScore).Error; err != nil {
				return err
			}

			// Recalculate total contest score
			var totalScore struct {
				Score int
			}
			if err := tx.Model(&models.UserProblemBestScore{}).
				Select("sum(score) as score").
				Where("user_id = ? AND contest_id = ?", sub.UserID, contestID).
				First(&totalScore).Error; err != nil {
				return err
			}

			// Record score change history
			history := models.ContestScoreHistory{
				UserID:                    sub.UserID,
				ContestID:                 contestID,
				ProblemID:                 sub.ProblemID,
				TotalScoreAfterChange:     totalScore.Score,
				LastEffectiveSubmissionID: sub.ID,
			}
			if err := tx.Create(&history).Error; err != nil {
				return err
			}
		}
		// If score is lower or equal, do nothing to the score or time.
		return nil
	})
}

// RecalculateScoresForUserProblem recalculates the best score for a given user/problem,
// and updates the total contest score if necessary. This is typically called after a
// submission is marked as invalid.
func RecalculateScoresForUserProblem(db *gorm.DB, userID, problemID, contestID, sourceSubmissionID string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// 1. Get the user's total score before any changes
		var oldTotalScore int
		// Find the most recent score history entry for the user in this contest
		if err := tx.Model(&models.ContestScoreHistory{}).
			Select("total_score_after_change").
			Where("user_id = ? AND contest_id = ?", userID, contestID).
			Order("created_at desc").
			Limit(1).
			Scan(&oldTotalScore).Error; err != nil {
			// If no record found, old score is 0. This is not an error.
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			oldTotalScore = 0
		}

		// Find the new best valid submission for the specific problem.
		// Highest score wins. For ties, earliest submission wins.
		var newBestSub models.Submission
		err := tx.Where("user_id = ? AND problem_id = ? AND is_valid = ?", userID, problemID, true).
			Order("score desc, created_at asc").
			First(&newBestSub).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// No valid submissions left for this problem, so delete the best score entry.
				if err := tx.Where("user_id = ? AND contest_id = ? AND problem_id = ?", userID, contestID, problemID).
					Delete(&models.UserProblemBestScore{}).Error; err != nil {
					return err
				}
			} else {
				// A different database error occurred.
				return err
			}
		} else {
			// A new best valid submission was found. Update or create the best score entry.
			bestScore := models.UserProblemBestScore{
				UserID:        userID,
				ContestID:     contestID,
				ProblemID:     problemID,
				Score:         newBestSub.Score,
				SubmissionID:  newBestSub.ID,
				LastScoreTime: newBestSub.CreatedAt,
			}
			// Use OnConflict to either create a new record or update the existing one based on the unique index.
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "contest_id"}, {Name: "problem_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"score", "submission_id", "last_score_time"}),
			}).Create(&bestScore).Error; err != nil {
				return err
			}
		}

		// Recalculate the new total score for the contest
		var newTotalScore int
		if err := tx.Model(&models.UserProblemBestScore{}).
			Select("COALESCE(SUM(score), 0)").
			Where("user_id = ? AND contest_id = ?", userID, contestID).
			Scan(&newTotalScore).Error; err != nil {
			return err
		}

		// If the total score has changed, create a new history record
		if newTotalScore != oldTotalScore {
			history := models.ContestScoreHistory{
				UserID:                    userID,
				ContestID:                 contestID,
				ProblemID:                 problemID, // The problem that triggered the change
				TotalScoreAfterChange:     newTotalScore,
				LastEffectiveSubmissionID: sourceSubmissionID, // The submission that was invalidated
			}
			if err := tx.Create(&history).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
