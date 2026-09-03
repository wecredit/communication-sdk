package audit

import (
	"sync"

	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

var (
	activeMu         sync.RWMutex
	activeDispatcher *Dispatcher
)

// Init constructs the database writer and bounded dispatcher. Callers provide
// the already-initialized write database and configured, manually provisioned
// table names; Init never creates or alters a table.
func Init(db *gorm.DB, inputTableName, outputTableName string, onError ErrorHandler) error {
	writer, err := NewWriter(db, inputTableName, outputTableName)
	if err != nil {
		return err
	}

	if onError == nil {
		onError = utils.Error
	}

	dispatcher, err := NewDispatcher(defaultWorkers, defaultBuffer, writer.Process, onError)
	if err != nil {
		return err
	}

	activeMu.Lock()
	activeDispatcher = dispatcher
	activeMu.Unlock()
	return nil
}

// TrySubmitInput copies and queues an input audit without blocking PUSH work.
func TrySubmitInput(input Input) bool {
	inputCopy := input
	return active().TrySubmit(Job{Input: &inputCopy})
}

// TrySubmitOutput copies and queues an output audit without blocking PUSH work.
func TrySubmitOutput(output Output) bool {
	outputCopy := output
	return active().TrySubmit(Job{Output: &outputCopy})
}

func active() *Dispatcher {
	activeMu.RLock()
	dispatcher := activeDispatcher
	activeMu.RUnlock()
	return dispatcher
}
