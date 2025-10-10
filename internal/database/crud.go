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

func GetAllContainers(db *gorm.DB, filters map[string]string) ([]models.Container, error) {
	var containers []models.Container
	query := db.Order("created_at desc")
	for key, value := range filters {
		query = query.Where(fmt.Sprintf("%s = ?", key), value)
	}
	if err := query.Find(&containers).Error; err != nil {
		return nil, err
	}
	return containers, nil
}

// Score & Leaderboard

type LeaderboardEntry struct {
	UserID           string         `json:"user_id"`
	Username         string         `json:"username"`
	Nickname         string         `json:"nickname"`
	AvatarURL        string         `json:"avatar_url"`
	TotalScore       int            `json:"total_score"`
	ProblemScores    map[string]int `json:"problem_scores"`
	lastScoreTime    time.Time
	registrationTime time.Time
}

// UserScoreHistoryPoint represents a single point in a user's score history for a contest.
type UserScoreHistoryPoint struct {
	Time      time.Time `json:"time"`
	Score     int       `json:"score"`
	ProblemID string    `json:"problem_id"`
}

func GetLeaderboard(db *gorm.DB, contestID string) ([]LeaderboardEntry, error) {
	// --- Step 1: Get all registered users and their registration time as a string ---
	type registeredUser struct {
		UserID           string
		Username         string
		Nickname         string
		AvatarURL        string
		RegistrationTime string // Read time as a string from DB
	}
	var users []registeredUser
	err := db.Table("contest_score_histories").
		Select("users.id as user_id, users.username, users.nickname, users.avatar_url, datetime(MIN(contest_score_histories.created_at)) as registration_time").
		Joins("join users on users.id = contest_score_histories.user_id").
		Where("contest_score_histories.contest_id = ?", contestID).
		Group("users.id, users.username, users.nickname, users.avatar_url").
		Scan(&users).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get registered users: %w", err)
	}

	// --- Step 2: Get all best scores for the contest ---
	type scoreRow struct {
		UserID        string
		ProblemID     string
		Score         int
		LastScoreTime time.Time
	}
	var scores []scoreRow
	err = db.Table("user_problem_best_scores").
		Select("user_id, problem_id, score, last_score_time").
		Where("contest_id = ?", contestID).
		Scan(&scores).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get scores: %w", err)
	}

	// --- Step 3: Combine users and scores ---
	resultsMap := make(map[string]*LeaderboardEntry)

	// Initialize map with all registered users, default score 0
	for _, user := range users {
		// Manually parse the time string. The format from SQLite's datetime() is "2006-01-02 15:04:05"
		regTime, parseErr := time.Parse("2006-01-02 15:04:05", user.RegistrationTime)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse registration time for user %s ('%s'): %w", user.UserID, user.RegistrationTime, parseErr)
		}

		avatarURL := user.AvatarURL
		if avatarURL != "" && !strings.HasPrefix(avatarURL, "http") {
			avatarURL = fmt.Sprintf("/api/v1/assets/avatars/%s", avatarURL)
		}
		resultsMap[user.UserID] = &LeaderboardEntry{
			UserID:           user.UserID,
			Username:         user.Username,
			Nickname:         user.Nickname,
			AvatarURL:        avatarURL,
			TotalScore:       0,
			ProblemScores:    make(map[string]int),
			lastScoreTime:    time.Time{}, // Zero value for time
			registrationTime: regTime,     // Use the parsed time object
		}
	}

	// Populate scores for users who have submitted
	for _, score := range scores {
		if entry, ok := resultsMap[score.UserID]; ok {
			entry.ProblemScores[score.ProblemID] = score.Score
			entry.TotalScore += score.Score
			if score.LastScoreTime.After(entry.lastScoreTime) {
				entry.lastScoreTime = score.LastScoreTime
			}
		}
	}

	// Convert map to slice
	var results []LeaderboardEntry
	for _, entry := range resultsMap {
		results = append(results, *entry)
	}

	// Sort the final slice
	sort.Slice(results, func(i, j int) bool {
		// Primary sort: Total Score (desc)
		if results[i].TotalScore != results[j].TotalScore {
			return results[i].TotalScore > results[j].TotalScore
		}

		// Scores are equal.
		// If score is 0, tie-break by registration time (asc - earlier is better).
		if results[i].TotalScore == 0 {
			return results[i].registrationTime.Before(results[j].registrationTime)
		}

		// If score is > 0, tie-break by last score time (asc - earlier is better).
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

func IsUserRegisteredForContest(db *gorm.DB, userID, contestID string) (bool, error) {
	var count int64
	err := db.Model(&models.ContestScoreHistory{}).
		Where("user_id = ? AND contest_id = ?", userID, contestID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
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

func GetBestScoresByUserID(db *gorm.DB, userID string) ([]models.UserProblemBestScore, error) {
	var scores []models.UserProblemBestScore
	err := db.Where("user_id = ?", userID).Find(&scores).Error
	return scores, err
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

			if err := createScoreHistory(tx, sub.UserID, contestID, sub.ProblemID, sub.ID); err != nil {
				return err
			}
		}
		// If score is lower or equal, do nothing to the score or time.
		return nil
	})
}

// Helper function to create score history to avoid repetition.
func createScoreHistory(tx *gorm.DB, userID, contestID, problemID, submissionID string) error {
	var totalScore struct {
		Score int
	}
	if err := tx.Model(&models.UserProblemBestScore{}).
		Select("sum(score) as score").
		Where("user_id = ? AND contest_id = ?", userID, contestID).
		First(&totalScore).Error; err != nil {
		return err
	}

	history := models.ContestScoreHistory{
		UserID:                    userID,
		ContestID:                 contestID,
		ProblemID:                 problemID,
		TotalScoreAfterChange:     totalScore.Score,
		LastEffectiveSubmissionID: submissionID,
	}
	return tx.Create(&history).Error
}

// RecalculateScoresForUserProblem recalculates the best score for a given user/problem,
// and updates the total contest score if necessary. This is typically called after a
// submission is marked as invalid.
func RecalculateScoresForUserProblem(db *gorm.DB, userID, problemID, contestID, sourceSubmissionID string, scoreMode string, maxPerformanceScore int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// For performance mode, we must recalculate for everyone, as the max performance might have changed.
		if scoreMode == "performance" {
			// First, find the new best valid submission for the user whose submission was invalidated.
			var newBestSub models.Submission
			err := tx.Where("user_id = ? AND problem_id = ? AND is_valid = ?", userID, problemID, true).
				Order("performance desc, created_at asc").
				First(&newBestSub).Error

			if errors.Is(err, gorm.ErrRecordNotFound) {
				// No valid submissions left for this user. Delete their best score record.
				if err := tx.Where("user_id = ? AND contest_id = ? AND problem_id = ?", userID, contestID, problemID).
					Delete(&models.UserProblemBestScore{}).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err // A different database error.
			} else {
				// A new best valid submission was found. Update the user's best score entry.
				// Score will be recalculated later. Just update performance and submission ID.
				// LastScoreTime is also updated because this is now their best attempt.
				if err := tx.Model(&models.UserProblemBestScore{}).
					Where("user_id = ? AND contest_id = ? AND problem_id = ?", userID, contestID, problemID).
					Updates(map[string]interface{}{
						"performance":     newBestSub.Performance,
						"submission_id":   newBestSub.ID,
						"last_score_time": newBestSub.CreatedAt,
					}).Error; err != nil {
					return err
				}
			}

			// Now, find the new global max performance for the problem.
			var newMaxPerformance struct {
				Performance float64
			}
			err = tx.Model(&models.UserProblemBestScore{}).
				Select("MAX(performance) as performance").
				Where("contest_id = ? AND problem_id = ?", contestID, problemID).
				Scan(&newMaxPerformance).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			// Get all user scores for this problem to recalculate them.
			var allUserScores []models.UserProblemBestScore
			if err := tx.Where("contest_id = ? AND problem_id = ?", contestID, problemID).Find(&allUserScores).Error; err != nil {
				return err
			}

			for _, userScore := range allUserScores {
				var newScore int
				if newMaxPerformance.Performance > 0 {
					if userScore.Performance == newMaxPerformance.Performance {
						newScore = maxPerformanceScore
					} else {
						newScore = int(float64(maxPerformanceScore) * userScore.Performance / newMaxPerformance.Performance)
					}
				} // If max performance is 0, score is 0.

				if userScore.Score != newScore {
					// Invalidation shouldn't update LastScoreTime for score changes.
					if err := tx.Model(&userScore).Update("score", newScore).Error; err != nil {
						return err
					}
					if err := createScoreHistory(tx, userScore.UserID, contestID, problemID, sourceSubmissionID); err != nil {
						return err
					}
				}
			}
			return nil
		}

		// --- Default "score" mode logic ---
		var oldTotalScore int
		if err := tx.Model(&models.ContestScoreHistory{}).
			Select("total_score_after_change").
			Where("user_id = ? AND contest_id = ?", userID, contestID).
			Order("created_at desc").
			Limit(1).
			Scan(&oldTotalScore).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			oldTotalScore = 0
		}

		var newBestSub models.Submission
		err := tx.Where("user_id = ? AND problem_id = ? AND is_valid = ?", userID, problemID, true).
			Order("score desc, created_at asc").
			First(&newBestSub).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Where("user_id = ? AND contest_id = ? AND problem_id = ?", userID, contestID, problemID).
					Delete(&models.UserProblemBestScore{}).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			bestScore := models.UserProblemBestScore{
				UserID:        userID,
				ContestID:     contestID,
				ProblemID:     problemID,
				Score:         newBestSub.Score,
				SubmissionID:  newBestSub.ID,
				LastScoreTime: newBestSub.CreatedAt,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "contest_id"}, {Name: "problem_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"score", "submission_id", "last_score_time"}),
			}).Create(&bestScore).Error; err != nil {
				return err
			}
		}

		var newTotalScore int
		if err := tx.Model(&models.UserProblemBestScore{}).
			Select("COALESCE(SUM(score), 0)").
			Where("user_id = ? AND contest_id = ?", userID, contestID).
			Scan(&newTotalScore).Error; err != nil {
			return err
		}

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

func UpdateScoresForPerformanceSubmission(db *gorm.DB, sub *models.Submission, contestID string, maxPerformanceScore int) error {
	// Performance score of 0 is ignored for initial scoring.
	if sub.Performance == 0 {
		return db.Model(sub).Update("performance", sub.Performance).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// First, update the submission's performance value. The score will be calculated and updated later in the transaction.
		if err := tx.Model(sub).UpdateColumns(map[string]interface{}{"performance": sub.Performance}).Error; err != nil {
			return err
		}

		// Get the current highest performance for this problem *before* this submission's impact is recorded in UserProblemBestScore.
		var currentMaxPerformance struct {
			Performance float64
		}
		err := tx.Model(&models.UserProblemBestScore{}).
			Select("MAX(performance) as performance").
			Where("contest_id = ? AND problem_id = ?", contestID, sub.ProblemID).
			Scan(&currentMaxPerformance).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Get this user's current best performance record.
		var userBestScore models.UserProblemBestScore
		err = tx.Where("user_id = ? AND contest_id = ? AND problem_id = ?", sub.UserID, contestID, sub.ProblemID).
			First(&userBestScore).Error
		isFirstSubmissionForUser := errors.Is(err, gorm.ErrRecordNotFound)

		// Only proceed if this is a new best performance for the user.
		if isFirstSubmissionForUser || sub.Performance > userBestScore.Performance {
			// Update or create the user's best performance record.
			// Score will be updated later. LastScoreTime is only updated on a score *increase*.
			userBestScore.UserID = sub.UserID
			userBestScore.ContestID = contestID
			userBestScore.ProblemID = sub.ProblemID
			userBestScore.Performance = sub.Performance
			userBestScore.SubmissionID = sub.ID
			if err := tx.Save(&userBestScore).Error; err != nil {
				return err
			}
		} else {
			// Not a new best for the user. Calculate their score based on current max and update the submission object, then we are done.
			score := 0
			if currentMaxPerformance.Performance > 0 {
				score = int(float64(maxPerformanceScore) * sub.Performance / currentMaxPerformance.Performance)
			}
			return tx.Model(sub).Update("score", score).Error
		}

		// --- Recalculate scores ---

		// Case 1: This submission sets a new global max performance.
		if sub.Performance > currentMaxPerformance.Performance {
			newMaxPerformance := sub.Performance
			// The submitter gets the max score.
			submitterNewScore := maxPerformanceScore
			if submitterNewScore > userBestScore.Score {
				// Score increased, update score and time.
				if err := tx.Model(&userBestScore).Updates(map[string]interface{}{"score": submitterNewScore, "last_score_time": sub.CreatedAt}).Error; err != nil {
					return err
				}
				if err := createScoreHistory(tx, sub.UserID, contestID, sub.ProblemID, sub.ID); err != nil {
					return err
				}
			} else {
				// Score did not increase (or it's the first submission), just update the score.
				if err := tx.Model(&userBestScore).Update("score", submitterNewScore).Error; err != nil {
					return err
				}
				if isFirstSubmissionForUser {
					if err := createScoreHistory(tx, sub.UserID, contestID, sub.ProblemID, sub.ID); err != nil {
						return err
					}
				}
			}

			// Update the submission object itself with the final score
			if err := tx.Model(sub).Update("score", submitterNewScore).Error; err != nil {
				return err
			}

			// Recalculate scores for all other users.
			var otherUserScores []models.UserProblemBestScore
			if err := tx.Where("contest_id = ? AND problem_id = ? AND user_id != ?", contestID, sub.ProblemID, sub.UserID).Find(&otherUserScores).Error; err != nil {
				return err
			}
			for _, otherUser := range otherUserScores {
				newScore := int(float64(maxPerformanceScore) * otherUser.Performance / newMaxPerformance)
				if otherUser.Score != newScore {
					// Score changed, update it. Do NOT update LastScoreTime.
					if err := tx.Model(&otherUser).Update("score", newScore).Error; err != nil {
						return err
					}
					if err := createScoreHistory(tx, otherUser.UserID, contestID, sub.ProblemID, sub.ID); err != nil {
						return err
					}
				}
			}
		} else { // Case 2: Not a new global max.
			// Calculate this user's score based on the existing max performance.
			newScore := int(float64(maxPerformanceScore) * sub.Performance / currentMaxPerformance.Performance)
			if newScore > userBestScore.Score {
				// Score increased, update score and time.
				if err := tx.Model(&userBestScore).Updates(map[string]interface{}{"score": newScore, "last_score_time": sub.CreatedAt}).Error; err != nil {
					return err
				}
				if err := createScoreHistory(tx, sub.UserID, contestID, sub.ProblemID, sub.ID); err != nil {
					return err
				}
			} else if isFirstSubmissionForUser {
				// First submission, not a record. Just set the score.
				if err := tx.Model(&userBestScore).Update("score", newScore).Error; err != nil {
					return err
				}
				if err := createScoreHistory(tx, sub.UserID, contestID, sub.ProblemID, sub.ID); err != nil {
					return err
				}
			}
			// Update the submission object itself with the final score
			if err := tx.Model(sub).Update("score", newScore).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
