package monitoring

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

var indianMobilePattern = regexp.MustCompile(`^[6-9][0-9]{9}$`)

func loadRuntimeConfig() RuntimeConfig {
	return ParseConfiguration(
		config.Configs.ZapCashMonitoringEnabled,
		config.Configs.ZapCashMonitorRecipients,
		config.Configs.ZapCashMonitorProfileJSON,
	)
}

// ParseConfiguration is exported so black-box tests can live under test/.
func ParseConfiguration(enabledRaw, recipientsRaw, profileRaw string) RuntimeConfig {
	if !strings.EqualFold(strings.TrimSpace(enabledRaw), "true") {
		return RuntimeConfig{}
	}

	result := RuntimeConfig{Enabled: true}
	seen := make(map[string]struct{})
	configured, rejected, duplicates := 0, 0, 0
	for _, raw := range strings.Split(recipientsRaw, ",") {
		candidate := normalizeMobile(raw)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		
		configured++
		if !indianMobilePattern.MatchString(candidate) {
			rejected++
			utils.Warn(fmt.Sprintf("ZapCash monitoring recipient rejected mobile=%s", maskMobile(candidate)))
			continue
		}

		if _, exists := seen[candidate]; exists {
			duplicates++
			continue
		}

		seen[candidate] = struct{}{}
		result.Recipients = append(result.Recipients, candidate)
	}

	if len(result.Recipients) == 0 {
		utils.Error(fmt.Errorf("ZapCash monitoring disabled: no valid recipients configured (configured=%d rejected=%d)", configured, rejected))
		return RuntimeConfig{}
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(profileRaw)), &result.Profile); err != nil {
		utils.Error(fmt.Errorf("ZapCash monitoring disabled: invalid ZAPCASH_MONITOR_PROFILE_JSON: %v", err))
		return RuntimeConfig{}
	}

	utils.Info(fmt.Sprintf("ZapCash monitoring configured enabled=true configured=%d active=%d rejected=%d duplicates=%d",
		configured, len(result.Recipients), rejected, duplicates))
	return result
}

func normalizeMobile(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "+91")
	if len(s) == 12 && strings.HasPrefix(s, "91") {
		s = strings.TrimPrefix(s, "91")
	}

	s = strings.TrimPrefix(s, "0")
	return s
}

func maskMobile(mobile string) string {
	if len(mobile) <= 4 {
		return "****"
	}

	return strings.Repeat("*", len(mobile)-4) + mobile[len(mobile)-4:]
}

// MaskMobile hides the sensitive portion of a mobile number in structured logs.
func MaskMobile(mobile string) string {
	return maskMobile(strings.TrimSpace(mobile))
}
