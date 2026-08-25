package configurationcache

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type templateVersionRow struct {
	TemplateVersion int64 `gorm:"column:TemplateVersion"`
}

type StageConfigurationVersions struct {
	LenderScheduleVersion int64 `gorm:"column:LenderScheduleVersion" json:"lenderScheduleVersion"`
	StageMappingVersion   int64 `gorm:"column:StageMappingVersion" json:"stageMappingVersion"`
}

// IncrementTemplateVersion increments the template version
func IncrementTemplateVersion(tx *gorm.DB, table string) (int64, error) {
	if tx == nil {
		return 0, errors.New("template version increment requires a transaction")
	}

	if table == "" {
		return 0, errors.New("configuration version table is required")
	}

	result := tx.Table(table).Where("Id = ?", 1).Updates(map[string]interface{}{
		"TemplateVersion": gorm.Expr("TemplateVersion + 1"),
		"UpdatedOn":       gorm.Expr("CURRENT_TIMESTAMP(3)"),
	})

	if result.Error != nil {
		return 0, fmt.Errorf("increment template version: %w", result.Error)
	}

	// check if the rows affected is 1
	if result.RowsAffected != 1 {
		return 0, fmt.Errorf("increment template version: expected one singleton row, updated %d", result.RowsAffected)
	}

	var row templateVersionRow
	if err := tx.Table(table).Select("TemplateVersion").Where("Id = ?", 1).First(&row).Error; err != nil {
		return 0, fmt.Errorf("read incremented template version: %w", err)
	}

	return row.TemplateVersion, nil
}

// IncrementStageConfigurationVersions increments the stage configuration versions
func IncrementStageConfigurationVersions(tx *gorm.DB, table string, scheduleChanged, mappingsChanged bool) (StageConfigurationVersions, error) {
	if tx == nil || table == "" {
		return StageConfigurationVersions{}, errors.New("configuration version increment requires a transaction and table")
	}

	updates := map[string]interface{}{}
	if scheduleChanged {
		updates["LenderScheduleVersion"] = gorm.Expr("LenderScheduleVersion + 1")
	}

	if mappingsChanged {
		updates["StageMappingVersion"] = gorm.Expr("StageMappingVersion + 1")
	}

	if len(updates) == 0 {
		return ReadStageConfigurationVersions(context.Background(), tx, table)
	}

	updates["UpdatedOn"] = gorm.Expr("CURRENT_TIMESTAMP(3)")
	result := tx.Table(table).Where("Id = ?", 1).Updates(updates)
	if result.Error != nil {
		return StageConfigurationVersions{}, fmt.Errorf("increment stage configuration versions: %w", result.Error)
	}

	if result.RowsAffected != 1 {
		return StageConfigurationVersions{}, fmt.Errorf("increment stage configuration versions: expected one singleton row, updated %d", result.RowsAffected)
	}

	return ReadStageConfigurationVersions(context.Background(), tx, table)
}

// ReadStageConfigurationVersions reads the stage configuration versions from the database
func ReadStageConfigurationVersions(ctx context.Context, db *gorm.DB, table string) (StageConfigurationVersions, error) {
	if db == nil || table == "" {
		return StageConfigurationVersions{}, errors.New("version read requires a database and table")
	}

	var versions StageConfigurationVersions
	if err := db.WithContext(ctx).Table(table).
		Select("LenderScheduleVersion, StageMappingVersion").Where("Id = ?", 1).First(&versions).Error; err != nil {
		return StageConfigurationVersions{}, fmt.Errorf("read stage configuration versions: %w", err)
	}

	return versions, nil
}

// ReadTemplateVersion reads the template version from the database
func ReadTemplateVersion(ctx context.Context, db *gorm.DB, table string) (int64, error) {
	if db == nil {
		return 0, errors.New("version read database is required")
	}

	// check if the table is required
	if table == "" {
		return 0, errors.New("configuration version table is required")
	}

	var row templateVersionRow
	if err := db.WithContext(ctx).Table(table).Select("TemplateVersion").Where("Id = ?", 1).First(&row).Error; err != nil {
		return 0, fmt.Errorf("read template version: %w", err)
	}

	// return the template version
	return row.TemplateVersion, nil
}
