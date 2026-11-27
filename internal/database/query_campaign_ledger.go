package database

import (
	"fmt"
	"strings"
	"time"

	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QueryCampaignLedgerDB returns the client ledger GORM DB.
// ZapCash → Core (DBZapCashV1). WeCredit Marketing ledger is not wired yet.
func QueryCampaignLedgerDB(client string) (*gorm.DB, error) {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case sdkHelper.ClientZapCash:
		if DBZapCashV1 == nil {
			return nil, fmt.Errorf("zapcash core database is not initialized for query campaign ledger")
		}
		return DBZapCashV1, nil
	case sdkHelper.ClientWeCredit:
		return nil, fmt.Errorf("wecredit marketing query campaign ledger is not configured yet")
	default:
		return nil, fmt.Errorf("unknown query campaign client %q", client)
	}
}

// ClaimQueryCampaignSending moves PROCESSING → SENDING on the client ledger DB.
func ClaimQueryCampaignSending(client string, ledgerInputID int64) (bool, error) {
	db, err := QueryCampaignLedgerDB(client)
	if err != nil {
		return false, err
	}

	now := time.Now().UTC()
	res := db.Exec(`
UPDATE CommQueryCampaignLedgerInput
SET Status = ?, UpdatedOn = ?
WHERE Id = ? AND Status = ?`,
		sdkHelper.LedgerSending, now, ledgerInputID, sdkHelper.LedgerProcessing)
	if res.Error != nil {
		return false, res.Error
	}

	return res.RowsAffected > 0, nil
}

// FinalizeQueryCampaignLedger sets terminal status from SENDING when possible and
// always attempts to insert the output row (insert-ignore on UNIQUE LedgerInputId+CreatedOn).
func FinalizeQueryCampaignLedger(row QueryCampaignLedgerOutputRow, terminalStatus string) error {
	db, err := QueryCampaignLedgerDB(row.Client)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res := db.Exec(`
UPDATE CommQueryCampaignLedgerInput
SET Status = ?, IsConsumed = ?, CommId = COALESCE(NULLIF(?, ''), CommId),
    TransactionId = COALESCE(NULLIF(?, ''), TransactionId),
    ErrorMessage = ?, UpdatedOn = ?
WHERE Id = ? AND Status = ?`,
		terminalStatus, sdkHelper.ConsumedTerminal,
		row.CommId, row.TransactionId, nullBlank(row.ResponseMessage), now,
		row.LedgerInputId, sdkHelper.LedgerSending)
	if res.Error != nil {
		return res.Error
	}
	return InsertQueryCampaignLedgerOutput(db, row)
}

// InsertQueryCampaignLedgerOutput inserts-or-ignores on UNIQUE (LedgerInputId, CreatedOn).
func InsertQueryCampaignLedgerOutput(db *gorm.DB, row QueryCampaignLedgerOutputRow) error {
	if db == nil {
		return fmt.Errorf("query campaign ledger database is nil")
	}

	if row.CreatedOn.IsZero() {
		row.CreatedOn = time.Now().UTC()
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).
		Table("CommQueryCampaignLedgerOutput").Create(&row).Error
}

func nullBlank(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	return s
}

// QueryCampaignLedgerOutputRow maps CommQueryCampaignLedgerOutput.
type QueryCampaignLedgerOutputRow struct {
	LedgerInputId     int64      `gorm:"column:LedgerInputId"`
	CampaignId        int64      `gorm:"column:CampaignId"`
	RunId             int64      `gorm:"column:RunId"`
	CommId            string     `gorm:"column:CommId"`
	EventId           string     `gorm:"column:EventId"`
	Client            string     `gorm:"column:Client"`
	Channel           string     `gorm:"column:Channel"`
	Vendor            string     `gorm:"column:Vendor"`
	ProcessName       string     `gorm:"column:ProcessName"`
	Mobile            string     `gorm:"column:Mobile"`
	TemplateReference string     `gorm:"column:TemplateReference"`
	IsSent            bool       `gorm:"column:IsSent"`
	Outcome           string     `gorm:"column:Outcome"`
	TransactionId     string     `gorm:"column:TransactionId"`
	ProviderMessageId string     `gorm:"column:ProviderMessageId"`
	ErrorCode         string     `gorm:"column:ErrorCode"`
	ResponseMessage   string     `gorm:"column:ResponseMessage"`
	HitAt             *time.Time `gorm:"column:HitAt"`
	CreatedOn         time.Time  `gorm:"column:CreatedOn"`
}

// LookupQueryCampaignLedgerMeta loads CampaignId/RunId for output rows from the client ledger.
func LookupQueryCampaignLedgerMeta(client string, ledgerInputID int64) (campaignID, runID int64, err error) {
	db, err := QueryCampaignLedgerDB(client)
	if err != nil {
		return 0, 0, err
	}

	var row struct {
		CampaignId int64
		RunId      int64
	}

	err = db.Table("CommQueryCampaignLedgerInput").
		Select("CampaignId, RunId").
		Where("Id = ?", ledgerInputID).
		Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return 0, 0, nil
	}

	return row.CampaignId, row.RunId, err
}
