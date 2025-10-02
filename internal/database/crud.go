package database

import (
	"errors"

	"github.com/ZJUSCT/CSOJ/internal/database/models"

	"gorm.io/gorm"
)

// User CRUD
func CreateUser(db *gorm.DB, user *models.User) error {
	return db.Create(user).Error
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
	if err := db.Preload("Containers").Where("id = ?", id).First(&sub).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func GetSubmissionsByUserID(db *gorm.DB, userID string) ([]models.Submission, error) {
	var subs []models.Submission
	if err := db.Where("user_id = ?", userID).Order("created_at desc").Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}

func GetAllSubmissions(db *gorm.DB) ([]models.Submission, error) {
	var subs []models.Submission
	if err := db.Order("created_at desc").Find(&subs).Error; err != nil {
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
	UserID     string `json:"user_id"`
	Username   string `json:"username"`
	Nickname   string `json:"nickname"`
	TotalScore int    `json:"total_score"`
}

func GetLeaderboard(db *gorm.DB, contestID string) ([]LeaderboardEntry, error) {
	var results []LeaderboardEntry
	err := db.Table("user_problem_best_scores").
		Select("users.id as user_id, users.username, users.nickname, SUM(user_problem_best_scores.score) as total_score").
		Joins("join users on users.id = user_problem_best_scores.user_id").
		Where("user_problem_best_scores.contest_id = ?", contestID).
		Group("user_problem_best_scores.user_id").
		Order("total_score desc").
		Scan(&results).Error
	return results, err
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

func UpdateScores(db *gorm.DB, sub *models.Submission, contestID string, newScore int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// 获取当前题目最高分
		var bestScore models.UserProblemBestScore
		err := tx.Where("user_id = ? AND contest_id = ? AND problem_id = ?", sub.UserID, contestID, sub.ProblemID).
			First(&bestScore).Error

		// 如果没有记录或者新分数更高
		if errors.Is(err, gorm.ErrRecordNotFound) || newScore > bestScore.Score {
			// 更新或创建最高分记录
			bestScore.UserID = sub.UserID
			bestScore.ContestID = contestID
			bestScore.ProblemID = sub.ProblemID
			bestScore.Score = newScore
			bestScore.SubmissionID = sub.ID
			if err := tx.Save(&bestScore).Error; err != nil {
				return err
			}

			// 重新计算比赛总分
			var totalScore struct {
				Score int
			}
			if err := tx.Model(&models.UserProblemBestScore{}).
				Select("sum(score) as score").
				Where("user_id = ? AND contest_id = ?", sub.UserID, contestID).
				First(&totalScore).Error; err != nil {
				return err
			}

			// 记录分数变化历史
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
		} else if newScore == bestScore.Score {
			// 同分提交，只更新提交ID
			bestScore.SubmissionID = sub.ID
			if err := tx.Save(&bestScore).Error; err != nil {
				return err
			}
		}
		// 如果分数更低，则什么都不做
		return nil
	})
}
