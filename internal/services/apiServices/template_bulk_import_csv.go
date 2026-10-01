package apiServices

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
)

var bulkHeaders = map[string]string{
	"Client": "Client", "Channel": "Channel", "Process": "Process", "Stage": "Stage", "Vendor": "Vendor",
	"Template Name": "TemplateName", "Image ID": "ImageId", "Image URL": "ImageUrl", "DLT Template ID": "DltTemplateId",
	"Template Entity ID": "TemplateEntityId", "Template Header": "TemplateHeader", "Is Active": "IsActive",
	"Template Text": "TemplateText", "Link": "Link", "Template Category": "TemplateCategory",
	"Provider Template Category": "ProviderTemplateCategory", "Variables": "TemplateVariables",
	"SMS Fallback Variables": "SmsFallbackVariables", "Subject": "Subject",
	"From Email": "FromEmail", "App ID": "AppId", "Language Code": "LanguageCode", "Campaign ID": "CampaignId", "CTA ID": "CtaId", "WABA Number": "WabaNumber",
}

type BulkTemplateRow struct {
	Row      int
	Template apiModels.Templatedetails
}

type BulkTemplateError struct {
	Row     int    `json:"row"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}
type BulkTemplateImportResult struct {
	DryRun             bool                `json:"dryRun"`
	TotalRows          int                 `json:"totalRows"`
	ValidRows          int                 `json:"validRows"`
	InsertedRows       int                 `json:"insertedRows"`
	FailedRows         int                 `json:"failedRows"`
	CreatedTemplateIds []int               `json:"createdTemplateIds"`
	Errors             []BulkTemplateError `json:"errors"`
}

func ParseBulkTemplateCSV(r io.Reader, maxRows int, maxBytes int64) ([]BulkTemplateRow, error) {
	limited := io.LimitReader(r, maxBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}
	if int64(len(b)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxBytes)
	}
	if !utf8.Valid(b) {
		return nil, errors.New("CSV must be valid UTF-8")
	}
	if len(b) >= 3 && string(b[:3]) == "\xef\xbb\xbf" {
		b = b[3:]
	}
	cr := csv.NewReader(strings.NewReader(string(b)))
	cr.FieldsPerRecord = -1
	headers, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV: %w", err)
	}
	seen := map[string]string{}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
		if headers[i] == "" {
			continue
		}
		canonical, ok := bulkHeaders[headers[i]]
		if !ok {
			accepted := make([]string, 0, len(bulkHeaders))
			for header := range bulkHeaders {
				accepted = append(accepted, header)
			}
			sort.Strings(accepted)
			return nil, fmt.Errorf("unknown header %q; accepted alternatives: %s", headers[i], strings.Join(accepted, ", "))
		}
		if previous, exists := seen[canonical]; exists {
			return nil, fmt.Errorf("duplicate headers %q and %q", previous, headers[i])
		}
		seen[canonical] = headers[i]
	}
	for _, required := range []string{"Client", "Channel", "Process", "Vendor"} {
		if _, ok := seen[required]; !ok {
			return nil, fmt.Errorf("missing required header %q", required)
		}
	}
	rows := make([]BulkTemplateRow, 0)
	for {
		record, readErr := cr.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("invalid CSV: %w", readErr)
		}
		physicalRow := len(rows) + 2
		if len(record) > 0 {
			physicalRow, _ = cr.FieldPos(0)
		}
		if len(record) < len(headers) {
			record = append(record, make([]string, len(headers)-len(record))...)
		}
		empty := true
		for _, v := range record {
			if strings.TrimSpace(v) != "" {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		for i, header := range headers {
			if header == "" && i < len(record) && strings.TrimSpace(record[i]) != "" {
				return nil, fmt.Errorf("empty header column %d contains data", i+1)
			}
		}
		if len(rows) >= maxRows {
			return nil, fmt.Errorf("row limit %d exceeded", maxRows)
		}
		t, err := bulkTemplateFromRecord(headers, record)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", physicalRow, err)
		}
		if _, hasActive := seen["IsActive"]; !hasActive {
			t.IsActive = true
		}
		if t.Channel == "WHATSAPP" && t.ProviderTemplateCategory != "" && t.ProviderTemplateCategory != "Marketing" && t.ProviderTemplateCategory != "Utility" {
			return nil, fmt.Errorf("row %d: Provider Template Category must be Marketing or Utility for WhatsApp", physicalRow)
		}
		if t.Channel == "WHATSAPP" && t.LanguageCode != "" && t.LanguageCode != "en" && t.LanguageCode != "en_US" {
			return nil, fmt.Errorf("row %d: Language Code must be en or en_US for WhatsApp", physicalRow)
		}
		rows = append(rows, BulkTemplateRow{Row: physicalRow, Template: t})
	}
	return rows, nil
}

func bulkTemplateFromRecord(headers, record []string) (apiModels.Templatedetails, error) {
	var t apiModels.Templatedetails
	for i, raw := range record {
		if i >= len(headers) {
			break
		}
		v := strings.TrimSpace(raw)
		switch bulkHeaders[headers[i]] {
		case "Client":
			t.Client = v
		case "Channel":
			t.Channel = v
		case "Process":
			t.Process = v
		case "Vendor":
			t.Vendor = v
		case "TemplateName":
			t.TemplateName = v
		case "ImageId":
			t.ImageId = v
		case "ImageUrl":
			t.ImageUrl = v
		case "TemplateHeader":
			t.TemplateHeader = v
		case "TemplateText":
			t.TemplateText = v
		case "Link":
			t.Link = v
		case "ProviderTemplateCategory":
			t.ProviderTemplateCategory = v
		case "TemplateVariables":
			t.TemplateVariables = v
		case "SmsFallbackVariables":
			t.SmsFallbackVariables = v
		case "Subject":
			t.Subject = v
		case "FromEmail":
			t.FromEmail = v
		case "AppId":
			t.AppId = v
		case "LanguageCode":
			t.LanguageCode = v
		case "CampaignId":
			t.CampaignId = v
		case "CtaId":
			t.CtaId = v
		case "WabaNumber":
			t.WabaNumber = v
		case "DltTemplateId":
			n, e := strconv.ParseInt(v, 10, 64)
			if v != "" && e != nil {
				return t, fmt.Errorf("DLT Template ID must be an integer")
			}
			t.DltTemplateId = n
		case "TemplateEntityId":
			n, e := strconv.ParseInt(v, 10, 64)
			if v != "" && e != nil {
				return t, fmt.Errorf("Template Entity ID must be an integer")
			}
			t.TemplateEntityId = n
		case "TemplateCategory":
			n, e := strconv.ParseInt(v, 10, 64)
			if v != "" && e != nil {
				return t, fmt.Errorf("Template Category must be an integer")
			}
			t.TemplateCategory = n
		case "IsActive":
			b, e := parseBulkBool(v)
			if e != nil {
				return t, e
			}
			t.IsActive = b
		case "Stage":
			if v != "" {
				f, e := parseBulkStage(v)
				if e != nil {
					return t, e
				}
				t.Stage = &f
			}
		}
	}
	normalizeTemplate(&t)
	return t, nil
}

func parseBulkBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("Is Active must be true or false")
	}
}
func parseBulkStage(v string) (float64, error) {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "-") {
		return 0, errors.New("stage cannot be negative")
	}
	parts := strings.Split(v, ".")
	if len(parts) > 2 || len(parts) == 2 && len(parts[1]) > 2 {
		return 0, errors.New("stage must have at most two decimal places")
	}
	if _, e := strconv.ParseUint(parts[0], 10, 64); e != nil {
		return 0, errors.New("stage must be a decimal")
	}
	if len(parts) == 2 {
		if _, e := strconv.ParseUint(parts[1], 10, 8); e != nil {
			return 0, errors.New("stage must be a decimal")
		}
	}
	f, e := strconv.ParseFloat(v, 64)
	if e != nil {
		return 0, errors.New("stage must be a decimal")
	}
	return f, nil
}
