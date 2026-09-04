package apiServices

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidSort is returned for unknown sortBy or invalid sortDir values.
var ErrInvalidSort = errors.New("invalid sort")

// listSortColumn maps a client sortBy token to a safe SQL expression (never raw input).
type listSortColumn struct {
	SQL         string
	DefaultDesc bool // updatedOn / createdOn / id default to DESC when sortDir omitted
}

func resolveListOrderClause(sortBy, sortDir, defaultClause string, allowlist map[string]listSortColumn) (string, error) {
	sortBy = strings.TrimSpace(sortBy)
	if sortBy == "" {
		// Still validate sortDir when provided so bad dirs are not silently ignored.
		if _, err := normalizeSortDir(sortDir, false); err != nil {
			return "", err
		}
		return defaultClause, nil
	}

	col, ok := allowlist[strings.ToLower(sortBy)]
	if !ok {
		return "", fmt.Errorf("%w: sortBy must be one of the allowlisted columns", ErrInvalidSort)
	}

	dir, err := normalizeSortDir(sortDir, col.DefaultDesc)
	if err != nil {
		return "", err
	}

	// Stable tie-breaker; never interpolate unsanitized input (allowlist only).
	return col.SQL + " " + dir + ", Id DESC", nil
}

func normalizeSortDir(raw string, defaultDesc bool) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "":
		if defaultDesc {
			return "DESC", nil
		}
		return "ASC", nil
	case "asc", "ascending":
		return "ASC", nil
	case "desc", "descending":
		return "DESC", nil
	default:
		return "", fmt.Errorf("%w: sortDir must be asc or desc", ErrInvalidSort)
	}
}

func templateSortAllowlist() map[string]listSortColumn {
	return map[string]listSortColumn{
		"client":       {SQL: "Client"},
		"process":      {SQL: "Process"},
		"channel":      {SQL: "Channel"},
		"vendor":       {SQL: "Vendor"},
		"templatename": {SQL: "TemplateName"},
		"stage":        {SQL: "Stage"},
		"updatedon":    {SQL: "COALESCE(UpdatedOn, CreatedOn)", DefaultDesc: true},
		"createdon":    {SQL: "CreatedOn", DefaultDesc: true},
		"id":           {SQL: "Id", DefaultDesc: true},
		"isactive":     {SQL: "IsActive"},
	}
}

func lenderScheduleSortAllowlist() map[string]listSortColumn {
	return map[string]listSortColumn{
		"lendername": {SQL: "LenderName"},
		"commtype":   {SQL: "CommType"},
		"stage":      {SQL: "Stage"},
		"interval":   {SQL: "`Interval`"},
		"updatedon":  {SQL: "UpdatedOn", DefaultDesc: true},
		"createdon":  {SQL: "CreatedOn", DefaultDesc: true},
		"id":         {SQL: "Id", DefaultDesc: true},
	}
}

func stageMappingSortAllowlist() map[string]listSortColumn {
	return map[string]listSortColumn{
		"lendername": {SQL: "LenderName"},
		"commtype":   {SQL: "CommType"},
		"stage":      {SQL: "Stage"},
		"substage":   {SQL: "SubStage"},
		"updatedon":  {SQL: "UpdatedOn", DefaultDesc: true},
		"createdon":  {SQL: "CreatedOn", DefaultDesc: true},
		"id":         {SQL: "Id", DefaultDesc: true},
	}
}

const (
	defaultLenderScheduleOrder = "Stage ASC, LenderName ASC, CommType ASC, Id ASC"
	defaultStageMappingOrder   = "Stage ASC, SubStage ASC, Id ASC"
)

// ResolveTemplateListOrder returns the ORDER BY clause for template listing.
func ResolveTemplateListOrder(sortBy, sortDir string, unrestricted bool) (string, error) {
	return resolveListOrderClause(sortBy, sortDir, TemplateListOrderClause(unrestricted), templateSortAllowlist())
}

// ResolveLenderScheduleListOrder returns the ORDER BY clause for lender schedule listing.
func ResolveLenderScheduleListOrder(sortBy, sortDir string) (string, error) {
	return resolveListOrderClause(sortBy, sortDir, defaultLenderScheduleOrder, lenderScheduleSortAllowlist())
}

// ResolveStageMappingListOrder returns the ORDER BY clause for stage mapping listing.
func ResolveStageMappingListOrder(sortBy, sortDir string) (string, error) {
	return resolveListOrderClause(sortBy, sortDir, defaultStageMappingOrder, stageMappingSortAllowlist())
}
