package sdkHelper

import (
	"fmt"
	"strings"
)

var smsVendors = map[string]bool{
	"TIMES": true, "SINCH": true, "PINNACLE": true, "VAPIO": true,
}
var waVendors = map[string]bool{
	"TIMES": true, "SINCH": true, "PINNACLE": true,
}

// AllowedDbTarget reports whether dbTargetId is known.
func AllowedDbTarget(id string) bool {
	switch strings.TrimSpace(id) {
	case DbTargetMarketingSQLServer, DbTargetZapCashMySQLV1, DbTargetCommunicationMySQL:
		return true
	default:
		return false
	}
}

// AllowedClient reports whether client is zapcash or wecredit.
func AllowedClient(client string) bool {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case ClientZapCash, ClientWeCredit:
		return true
	default:
		return false
	}
}

// DbTargetAllowedForClient enforces Client RBAC on DbTargetId.
func DbTargetAllowedForClient(dbTargetID, client string) bool {
	if !AllowedDbTarget(dbTargetID) || !AllowedClient(client) {
		return false
	}
	c := strings.ToLower(strings.TrimSpace(client))
	switch dbTargetID {
	case DbTargetMarketingSQLServer:
		return c == ClientWeCredit
	case DbTargetZapCashMySQLV1:
		return c == ClientZapCash
	case DbTargetCommunicationMySQL:
		return c == ClientWeCredit || c == ClientZapCash
	default:
		return false
	}
}

// ClientMatches reports whether a row client equals the campaign Client.
func ClientMatches(rowClient, campaignClient string) bool {
	return strings.ToLower(strings.TrimSpace(rowClient)) == strings.ToLower(strings.TrimSpace(campaignClient)) &&
		AllowedClient(campaignClient)
}

// MapRowAliases maps a lowercase-alias column map into a CampaignRow and validates.
func MapRowAliases(cols map[string]string, expectedChannel, campaignClient string) (CampaignRow, error) {
	get := func(k string) string { return strings.TrimSpace(cols[strings.ToLower(k)]) }
	row := CampaignRow{
		Client:            strings.ToLower(get("client")),
		Channel:           strings.ToUpper(get("channel")),
		Vendor:            strings.ToUpper(get("vendor")),
		ProcessName:       strings.ToUpper(get("processname")),
		Mobile:            get("mobile"),
		TemplateReference: get("templatereference"),
		AppId:             get("appid"),
		VariableValues:    get("variablevalues"),
	}
	if row.Client == "" || row.Channel == "" || row.Vendor == "" || row.ProcessName == "" || row.TemplateReference == "" {
		return row, fmt.Errorf("missing required identity columns")
	}
	if strings.EqualFold(row.Channel, "EMAIL") {
		return row, fmt.Errorf("EMAIL channel is not supported")
	}
	if !strings.EqualFold(row.Channel, expectedChannel) {
		return row, fmt.Errorf("row channel %q does not match campaign channel %q", row.Channel, expectedChannel)
	}
	if !ClientMatches(row.Client, campaignClient) {
		return row, fmt.Errorf("row client %q does not match campaign client %q", row.Client, campaignClient)
	}
	switch row.Channel {
	case ChannelSMS:
		if row.Mobile == "" {
			return row, fmt.Errorf("mobile is required for SMS")
		}
		if !smsVendors[row.Vendor] {
			return row, fmt.Errorf("vendor %q is not allowed for SMS", row.Vendor)
		}
	case ChannelWhatsApp:
		if row.Mobile == "" {
			return row, fmt.Errorf("mobile is required for WHATSAPP")
		}
		if !waVendors[row.Vendor] {
			return row, fmt.Errorf("vendor %q is not allowed for WHATSAPP", row.Vendor)
		}
	default:
		return row, fmt.Errorf("channel %q is not supported in v1", row.Channel)
	}
	return row, nil
}
