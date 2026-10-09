package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
	"gorm.io/gorm"
)

const auditInsertTimeout = 2 * time.Second

// auditInsertSlots caps in-flight audit inserts at the Core pool size so a
// slow database cannot pile up goroutines or extra connections.
var auditInsertSlots = make(chan struct{}, zapCashV1MaxOpenConns)

var (
	commInputOutputWriteWeCredit string
	commInputOutputWriteZapCash  string
	zapCashV1InputOutputWrite    string
)

// SetChannelAuditFlags stores the Input/Output write toggles. config calls this
// at startup so this package does not import config.
func SetChannelAuditFlags(weCreditFlag, zapCashFlag, v1Flag string) {
	commInputOutputWriteWeCredit = weCreditFlag
	commInputOutputWriteZapCash = zapCashFlag
	zapCashV1InputOutputWrite = v1Flag
}

// CommWriteEnabled reports whether this client still inserts channel
// Input/Output rows into the communication database. WeCredit and ZapCash
// stop only when their flag, trimmed, equals "false" in any case. Any other
// value, including missing, keeps writing. Other clients always write.
func CommWriteEnabled(client, weCreditFlag, zapCashFlag string) bool {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case variables.WeCredit:
		return !flagIsFalse(weCreditFlag)
	case variables.ZapCash:
		return !flagIsFalse(zapCashFlag)
	default:
		return true
	}
}

// ZapCashV1WriteEnabled is on only when the flag, trimmed, equals "true"
// in any case.
func ZapCashV1WriteEnabled(flag string) bool {
	return strings.EqualFold(strings.TrimSpace(flag), "true")
}

// ChannelAuditDestinations chooses the communication database and Core.
// Core applies only to ZapCash.
func ChannelAuditDestinations(client, weCreditFlag, zapCashFlag, v1Flag string) (writeComm, writeCore bool) {
	writeComm = CommWriteEnabled(client, weCreditFlag, zapCashFlag)
	writeCore = strings.EqualFold(strings.TrimSpace(client), variables.ZapCash) && ZapCashV1WriteEnabled(v1Flag)
	return writeComm, writeCore
}

func flagIsFalse(flag string) bool {
	return strings.EqualFold(strings.TrimSpace(flag), "false")
}

// RunChannelAuditInsert runs one audit insert. A real error is logged and
// swallowed so the provider send, SQS ACK, and API response continue.
// A nil insert is a silent skip.
func RunChannelAuditInsert(client, table string, insert func() error) {
	if insert == nil {
		return
	}
	if err := insert(); err != nil {
		utils.Error(fmt.Errorf("audit_insert_failed client=%s table=%s: %v", strings.TrimSpace(client), strings.TrimSpace(table), err))
	}
}

// InsertRow writes one channel Input/Output row to each enabled destination.
// The insert runs in the background, so a slow or unreachable audit database
// does not hold the API response or the SQS acknowledgement. A disabled toggle
// or a nil pool is a silent skip. Callers that run inside nurture must still
// require DBtechWrite != nil before calling this, so that process does not insert.
func InsertRow(client, table string, row map[string]interface{}) {
	writeComm, writeCore := ChannelAuditDestinations(
		client,
		commInputOutputWriteWeCredit,
		commInputOutputWriteZapCash,
		zapCashV1InputOutputWrite,
	)
	commDB := DBtechWrite
	coreDB := DBZapCashV1
	if (!writeComm || commDB == nil) && (!writeCore || coreDB == nil) {
		return
	}
	copied := copyAuditRow(row)
	if !enqueueAuditInsert(func() {
		if writeComm && commDB != nil {
			RunChannelAuditInsert(client, table, func() error {
				return insertAuditWithTimeout(table, commDB, copied)
			})
		}
		if writeCore && coreDB != nil {
			RunChannelAuditInsert(client, table, func() error {
				return insertAuditWithTimeout(table, coreDB, copied)
			})
		}
	}) {
		utils.Error(fmt.Errorf("audit_insert_failed client=%s table=%s: audit insert busy", strings.TrimSpace(client), strings.TrimSpace(table)))
	}
}

func insertAuditWithTimeout(table string, db *gorm.DB, row map[string]interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), auditInsertTimeout)
	defer cancel()
	return InsertData(table, db.WithContext(ctx), row)
}

func copyAuditRow(row map[string]interface{}) map[string]interface{} {
	if row == nil {
		return nil
	}
	copied := make(map[string]interface{}, len(row))
	for key, value := range row {
		copied[key] = value
	}
	return copied
}

// enqueueAuditInsert starts write without waiting. It returns false when every
// slot is already in use.
func enqueueAuditInsert(write func()) bool {
	if write == nil {
		return false
	}
	select {
	case auditInsertSlots <- struct{}{}:
	default:
		return false
	}
	go func() {
		defer func() { <-auditInsertSlots }()
		defer func() {
			if recovered := recover(); recovered != nil {
				utils.Error(fmt.Errorf("audit_insert_failed: %v", recovered))
			}
		}()
		write()
	}()
	return true
}

// WillWrite reports whether InsertRow would attempt at least one insert for
// this client with the current flags and pools.
func WillWrite(client string) bool {
	writeComm, writeCore := ChannelAuditDestinations(
		client,
		commInputOutputWriteWeCredit,
		commInputOutputWriteZapCash,
		zapCashV1InputOutputWrite,
	)
	if writeComm && DBtechWrite != nil {
		return true
	}
	return writeCore && DBZapCashV1 != nil
}
