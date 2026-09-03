package audit

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var safeTableName = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// Writer persists PUSH audit records to manually provisioned input and output
// tables. It performs no migration or schema mutation.
type Writer struct {
	db              *gorm.DB
	inputTableName  string
	outputTableName string
	now             func() time.Time
}

func NewWriter(db *gorm.DB, inputTableName, outputTableName string) (*Writer, error) {
	if db == nil {
		return nil, errors.New("PUSH audit database is required")
	}

	inputTableName = strings.TrimSpace(inputTableName)
	if !validTableName(inputTableName) {
		return nil, errors.New("PUSH input audit table name is invalid")
	}

	outputTableName = strings.TrimSpace(outputTableName)
	if !validTableName(outputTableName) {
		return nil, errors.New("PUSH output audit table name is invalid")
	}

	return &Writer{
		db:              db,
		inputTableName:  inputTableName,
		outputTableName: outputTableName,
		now:             time.Now,
	}, nil
}

// Process implements Processor for use by Dispatcher.
func (w *Writer) Process(job Job) error {
	if w == nil || w.db == nil {
		return errors.New("PUSH audit writer is not initialized")
	}

	if !validJob(job) {
		return errors.New("PUSH audit job must contain exactly one record")
	}

	if job.Input != nil {
		input := *job.Input
		if input.CreatedOn.IsZero() {
			input.CreatedOn = w.now().UTC()
		}

		if err := w.db.Session(&gorm.Session{NewDB: true}).
			Table(w.inputTableName).Clauses(clause.OnConflict{DoNothing: true}).Create(&input).Error; err != nil {
			return fmt.Errorf("insert PUSH input audit: %w", err)
		}

		return nil
	}

	output := *job.Output
	if output.CreatedOn.IsZero() {
		output.CreatedOn = w.now().UTC()
	}

	if err := w.db.Session(&gorm.Session{NewDB: true}).
		Table(w.outputTableName).Clauses(clause.OnConflict{DoNothing: true}).Create(&output).Error; err != nil {
		return fmt.Errorf("insert PUSH output audit: %w", err)
	}

	return nil
}

func validTableName(tableName string) bool {
	return tableName != "" && safeTableName.MatchString(tableName)
}
