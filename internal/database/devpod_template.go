package database

import (
	"regexp"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"gorm.io/gorm"
)

var templateIDRE = regexp.MustCompile(`^[a-z0-9-]{1,6}$`)

// ValidateDevPodTemplateID returns true if id is a valid DevPod template ID.
func ValidateDevPodTemplateID(id string) bool {
	return templateIDRE.MatchString(id)
}

func ListDevPodTemplates(db *gorm.DB) ([]models.DevPodTemplate, error) {
	var rows []models.DevPodTemplate
	err := db.Order("id").Find(&rows).Error
	return rows, err
}

func GetDevPodTemplate(db *gorm.DB, id string) (*models.DevPodTemplate, error) {
	var tpl models.DevPodTemplate
	if err := db.Where("id = ?", id).First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func CreateDevPodTemplate(db *gorm.DB, tpl *models.DevPodTemplate) error {
	return db.Create(tpl).Error
}

func UpdateDevPodTemplate(db *gorm.DB, tpl *models.DevPodTemplate) error {
	return db.Save(tpl).Error
}

func DeleteDevPodTemplate(db *gorm.DB, id string) error {
	return db.Where("id = ?", id).Delete(&models.DevPodTemplate{}).Error
}
