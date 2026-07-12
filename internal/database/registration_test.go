package database_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"gorm.io/gorm"
)

func newRegistrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Init(":memory:")
	if err != nil {
		t.Fatalf("init database: %v", err)
	}
	return db
}

func historyCount(t *testing.T, db *gorm.DB, userID, contestID string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&models.ContestScoreHistory{}).
		Where("user_id = ? AND contest_id = ?", userID, contestID).
		Count(&count).Error; err != nil {
		t.Fatalf("count score history: %v", err)
	}
	return count
}

func TestCreateApprovedRegistrationCreatesScoreEvent(t *testing.T) {
	db := newRegistrationTestDB(t)
	reg := &models.ContestRegistration{ID: "reg-approved", UserID: "u1", ContestID: "c1", Status: "approved"}
	if err := database.CreateRegistration(db, reg); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if got := historyCount(t, db, "u1", "c1"); got != 1 {
		t.Fatalf("history count = %d, want 1", got)
	}
	history, err := database.GetScoreHistoryForUser(db, "c1", "u1")
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(history) != 1 || history[0].Score != 0 || history[0].ProblemID != "" {
		t.Fatalf("registration history = %#v, want one zero-score Registration point", history)
	}
}

func TestRejectedToApprovedCreatesOneScoreEventAndEnforcesStatus(t *testing.T) {
	db := newRegistrationTestDB(t)
	reg := &models.ContestRegistration{ID: "reg-review", UserID: "u2", ContestID: "c2", Status: "rejected"}
	if err := database.CreateRegistration(db, reg); err != nil {
		t.Fatalf("create rejected registration: %v", err)
	}
	if got := historyCount(t, db, "u2", "c2"); got != 0 {
		t.Fatalf("rejected registration history count = %d, want 0", got)
	}

	if err := database.UpdateRegistrationStatus(db, reg.ID, "approved", "admin"); err != nil {
		t.Fatalf("approve registration: %v", err)
	}
	if got := historyCount(t, db, "u2", "c2"); got != 1 {
		t.Fatalf("approved registration history count = %d, want 1", got)
	}
	approved, err := database.IsUserApprovedForContest(db, "u2", "c2")
	if err != nil || !approved {
		t.Fatalf("approved = %v, err = %v; want true, nil", approved, err)
	}

	if err := database.UpdateRegistrationStatus(db, reg.ID, "rejected", "admin"); err != nil {
		t.Fatalf("reject registration: %v", err)
	}
	approved, err = database.IsUserApprovedForContest(db, "u2", "c2")
	if err != nil || approved {
		t.Fatalf("approved after rejection = %v, err = %v; want false, nil", approved, err)
	}
	if err := database.UpdateRegistrationStatus(db, reg.ID, "approved", "admin"); err != nil {
		t.Fatalf("reapprove registration: %v", err)
	}
	if got := historyCount(t, db, "u2", "c2"); got != 1 {
		t.Fatalf("history count after reapproval = %d, want 1", got)
	}
}

func TestLeaderboardExcludesModernRejectedRegistration(t *testing.T) {
	db := newRegistrationTestDB(t)
	user := &models.User{ID: "u3", Username: "user3", Nickname: "User 3"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	reg := &models.ContestRegistration{ID: "reg-board", UserID: user.ID, ContestID: "c3", Status: "approved"}
	if err := database.CreateRegistration(db, reg); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	entries, err := database.GetLeaderboard(db, "c3", "")
	if err != nil || len(entries) != 1 {
		t.Fatalf("approved leaderboard entries = %d, err = %v; want 1, nil", len(entries), err)
	}
	if err := database.UpdateRegistrationStatus(db, reg.ID, "rejected", "admin"); err != nil {
		t.Fatalf("reject registration: %v", err)
	}
	entries, err = database.GetLeaderboard(db, "c3", "")
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected leaderboard entries = %d, err = %v; want 0, nil", len(entries), err)
	}
}

func TestLegacyScoreHistoryRegistrationStillAllowed(t *testing.T) {
	db := newRegistrationTestDB(t)
	if err := database.RegisterForContest(db, "legacy-user", "legacy-contest"); err != nil {
		t.Fatalf("legacy registration: %v", err)
	}
	approved, err := database.IsUserApprovedForContest(db, "legacy-user", "legacy-contest")
	if err != nil || !approved {
		t.Fatalf("legacy approved = %v, err = %v; want true, nil", approved, err)
	}
}

func TestPendingRegistrationIsNotApproved(t *testing.T) {
	db := newRegistrationTestDB(t)
	reg := &models.ContestRegistration{ID: "reg-pending", UserID: "pending-user", ContestID: "pending-contest", Status: "pending"}
	if err := database.CreateRegistration(db, reg); err != nil {
		t.Fatalf("create pending registration: %v", err)
	}
	approved, err := database.IsUserApprovedForContest(db, reg.UserID, reg.ContestID)
	if err != nil || approved {
		t.Fatalf("pending approved = %v, err = %v; want false, nil", approved, err)
	}
	if got := historyCount(t, db, reg.UserID, reg.ContestID); got != 0 {
		t.Fatalf("pending registration history count = %d, want 0", got)
	}
}

func TestInitBackfillsExistingApprovedRegistration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "csoj.db")
	db, err := database.Init(path)
	if err != nil {
		t.Fatalf("initial init: %v", err)
	}
	reviewedAt := time.Date(2026, 7, 12, 9, 30, 0, 0, time.UTC)
	reg := &models.ContestRegistration{
		ID:         "reg-existing",
		UserID:     "existing-user",
		ContestID:  "existing-contest",
		Status:     "approved",
		ReviewedAt: &reviewedAt,
	}
	// Insert directly to reproduce data written by the old implementation.
	if err := db.Create(reg).Error; err != nil {
		t.Fatalf("insert legacy approved registration: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close initial database: %v", err)
	}

	db, err = database.Init(path)
	if err != nil {
		t.Fatalf("restart init: %v", err)
	}
	if got := historyCount(t, db, reg.UserID, reg.ContestID); got != 1 {
		t.Fatalf("backfilled history count = %d, want 1", got)
	}
	history, err := database.GetScoreHistoryForUser(db, reg.ContestID, reg.UserID)
	if err != nil {
		t.Fatalf("get backfilled history: %v", err)
	}
	if len(history) != 1 || !history[0].Time.Equal(reviewedAt) {
		t.Fatalf("backfilled history = %#v, want event at %s", history, reviewedAt)
	}

	if err := database.UpdateRegistrationStatus(db, reg.ID, "approved", "admin"); err != nil {
		t.Fatalf("repeat approval: %v", err)
	}
	if got := historyCount(t, db, reg.UserID, reg.ContestID); got != 1 {
		t.Fatalf("history count after repeated approval = %d, want 1", got)
	}
}
