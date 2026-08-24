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
