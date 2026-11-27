package sdkHelper

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RunPreview streams the author query and stops after PreviewMaxRows validated
// rows (default 100). It does not wrap the SQL in LIMIT/TOP and does not count
// the full result set.
func RunPreview(ctx context.Context, cfg RunConfig) (PreviewResult, error) {
	started := time.Now()
	var out PreviewResult
	if err := CheckSingleSelect(cfg.SqlText); err != nil {
		out.Error = err.Error()
		return out, err
	}
	if cfg.DB == nil {
		return out, fmt.Errorf("database is not configured for dbTargetId %s", cfg.DbTargetID)
	}
	if !DbTargetAllowedForClient(cfg.DbTargetID, cfg.Client) {
		return out, fmt.Errorf("dbTargetId %s is not allowed for client %s", cfg.DbTargetID, cfg.Client)
	}
	maxRows := cfg.PreviewMaxRows
	if maxRows <= 0 {
		maxRows = DefaultPreviewMaxRows
	}

	timeout := cfg.EffectiveTimeout()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	applyDBTimeout(runCtx, cfg)

	rows, err := cfg.DB.QueryContext(runCtx, cfg.SqlText)
	if err != nil {
		out.Error = err.Error()
		out.Duration = time.Since(started)
		return out, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		out.Error = err.Error()
		return out, err
	}
	lowerCols := make([]string, len(columns))
	for i, c := range columns {
		lowerCols[i] = strings.ToLower(strings.TrimSpace(c))
	}

	raw := make([]sql.NullString, len(columns))
	ptrs := make([]interface{}, len(columns))
	for i := range raw {
		ptrs[i] = &raw[i]
	}

	for rows.Next() {
		if out.SampleRows >= maxRows {
			break
		}
		if err := rows.Scan(ptrs...); err != nil {
			out.Error = err.Error()
			out.Duration = time.Since(started)
			return out, err
		}
		colMap := map[string]string{}
		for i, name := range lowerCols {
			if raw[i].Valid {
				colMap[name] = strings.TrimSpace(raw[i].String)
			} else {
				colMap[name] = ""
			}
		}
		campaignRow, mapErr := MapRowAliases(colMap, cfg.Channel, cfg.Client)
		if mapErr != nil {
			out.Error = mapErr.Error()
			out.Duration = time.Since(started)
			return out, mapErr
		}
		if cfg.Templates != nil {
			ok, terr := cfg.Templates.TemplateApproved(runCtx, campaignRow.Client, campaignRow.Channel, campaignRow.ProcessName, campaignRow.TemplateReference)
			if terr != nil {
				out.Error = terr.Error()
				out.Duration = time.Since(started)
				return out, terr
			}
			if !ok {
				err := fmt.Errorf("templateReference %q is missing or not approved", campaignRow.TemplateReference)
				out.Error = err.Error()
				out.Duration = time.Since(started)
				return out, err
			}
		}
		out.SampleRows++
	}
	if err := rows.Err(); err != nil {
		out.Error = err.Error()
		out.Duration = time.Since(started)
		return out, err
	}
	out.Duration = time.Since(started)
	return out, nil
}

// RunMaterialize streams validated rows into sink in batches. Caller owns the
// outer transaction. On abort threshold, returns an error with ROW_COUNT_EXCEEDS_APPROVED.
func RunMaterialize(ctx context.Context, cfg RunConfig, sink MaterializeSink, batchSize int) (MaterializeResult, error) {
	started := time.Now()
	var out MaterializeResult
	if err := CheckSingleSelect(cfg.SqlText); err != nil {
		return out, err
	}
	if cfg.DB == nil {
		return out, fmt.Errorf("database is not configured for dbTargetId %s", cfg.DbTargetID)
	}
	if !DbTargetAllowedForClient(cfg.DbTargetID, cfg.Client) {
		return out, fmt.Errorf("dbTargetId %s is not allowed for client %s", cfg.DbTargetID, cfg.Client)
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	hardCap := cfg.MaterializeHardCap
	if hardCap <= 0 {
		hardCap = DefaultMaterializeHardCap
	}
	abortAt := hardCap
	if cfg.AbortThreshold > 0 && cfg.AbortThreshold < abortAt {
		abortAt = cfg.AbortThreshold
	}

	timeout := cfg.EffectiveTimeout()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	applyDBTimeout(runCtx, cfg)

	rows, err := cfg.DB.QueryContext(runCtx, cfg.SqlText)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return out, err
	}
	lowerCols := make([]string, len(columns))
	for i, c := range columns {
		lowerCols[i] = strings.ToLower(strings.TrimSpace(c))
	}
	raw := make([]sql.NullString, len(columns))
	ptrs := make([]interface{}, len(columns))
	for i := range raw {
		ptrs[i] = &raw[i]
	}

	batch := make([]CampaignRow, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := sink.InsertBatch(runCtx, batch); err != nil {
			return err
		}
		out.RowsInserted += len(batch)
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return out, err
		}
		colMap := map[string]string{}
		for i, name := range lowerCols {
			if raw[i].Valid {
				colMap[name] = strings.TrimSpace(raw[i].String)
			}
		}
		out.RowsSelected++
		if out.RowsSelected > abortAt {
			_ = rows.Close()
			return out, fmt.Errorf("ROW_COUNT_EXCEEDS_APPROVED: streamed rows exceeded approved expected rows (+margin) or hard cap; pause → preview → approve (new ApprovedExpectedRows) → enable")
		}
		campaignRow, mapErr := MapRowAliases(colMap, cfg.Channel, cfg.Client)
		if mapErr != nil {
			out.RowsSkippedInvalid++
			continue
		}
		if cfg.Templates != nil {
			ok, terr := cfg.Templates.TemplateApproved(runCtx, campaignRow.Client, campaignRow.Channel, campaignRow.ProcessName, campaignRow.TemplateReference)
			if terr != nil {
				return out, terr
			}
			if !ok {
				out.RowsSkippedInvalid++
				continue
			}
		}
		batch = append(batch, campaignRow)
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return out, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if err := flush(); err != nil {
		return out, err
	}
	out.Duration = time.Since(started)
	return out, nil
}

func applyDBTimeout(ctx context.Context, cfg RunConfig) {
	// Best-effort session timeouts; dialects differ. Failures are ignored.
	sec := int(cfg.EffectiveTimeout().Seconds())
	if sec < 1 {
		return
	}
	switch cfg.DbTargetID {
	case DbTargetMarketingSQLServer:
		_, _ = cfg.DB.ExecContext(ctx, fmt.Sprintf("SET LOCK_TIMEOUT %d", sec*1000))
	case DbTargetZapCashMySQLV1, DbTargetCommunicationMySQL:
		_, _ = cfg.DB.ExecContext(ctx, fmt.Sprintf("SET SESSION max_execution_time = %d", sec*1000))
	}
}
