package middleware

import (
	"errors"
	"strings"
)

var ErrScopeForbidden = errors.New("access denied for this client")

const defaultSuperAdminRoles = "marketing"
const defaultClientRolePrefix = "marketing_"

type CommAdminScope struct {
	AllowedClients []string
	Unrestricted   bool
}

type CommScopeConfig struct {
	SuperAdminRoles  []string
	ClientRolePrefix string
}

func NewCommScopeConfig(superAdminRolesCSV, clientRolePrefix string) CommScopeConfig {
	prefix := strings.ToLower(strings.TrimSpace(clientRolePrefix))
	if prefix == "" {
		prefix = defaultClientRolePrefix
	}

	superAdminRoles := parseCommaSeparated(superAdminRolesCSV)
	if len(superAdminRoles) == 0 {
		superAdminRoles = parseCommaSeparated(defaultSuperAdminRoles)
	}

	return CommScopeConfig{
		SuperAdminRoles:  superAdminRoles,
		ClientRolePrefix: prefix,
	}
}

func ResolveCommAdminScope(role, username string, cfg CommScopeConfig) (CommAdminScope, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	username = strings.TrimSpace(username)

	if scope, ok := scopeFromRole(role, cfg); ok {
		return scope, nil
	}

	if role == "" {
		if scope, ok := scopeFromUsername(username, cfg); ok {
			return scope, nil
		}
	}

	return CommAdminScope{}, ErrScopeForbidden
}

func scopeFromRole(role string, cfg CommScopeConfig) (CommAdminScope, bool) {
	if role == "" {
		return CommAdminScope{}, false
	}

	for _, superAdminRole := range cfg.SuperAdminRoles {
		if role == superAdminRole {
			return CommAdminScope{Unrestricted: true}, true
		}
	}

	if strings.HasPrefix(role, cfg.ClientRolePrefix) {
		client := strings.ToLower(strings.TrimPrefix(role, cfg.ClientRolePrefix))
		if client == "" {
			return CommAdminScope{}, false
		}

		return CommAdminScope{AllowedClients: []string{client}}, true
	}

	return CommAdminScope{}, false
}

func scopeFromUsername(username string, cfg CommScopeConfig) (CommAdminScope, bool) {
	if username == "" || !strings.Contains(username, "@") {
		return CommAdminScope{}, false
	}

	localPart := strings.ToLower(strings.SplitN(username, "@", 2)[0])
	if localPart == "" {
		return CommAdminScope{}, false
	}

	return scopeFromRole(localPart, cfg)
}

func parseCommaSeparated(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value != "" {
			values = append(values, value)
		}
	}

	return values
}

func clientAllowed(scope CommAdminScope, client string) bool {
	if scope.Unrestricted {
		return true
	}

	client = strings.ToLower(strings.TrimSpace(client))
	if client == "" {
		return false
	}

	for _, allowed := range scope.AllowedClients {
		if client == allowed {
			return true
		}
	}

	return false
}
