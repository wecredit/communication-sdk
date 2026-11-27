package sdkHelper

import (
	"fmt"
	"strings"
)

// CheckSingleSelect performs a lightweight single-statement SELECT/WITH check.
// It is not a full SQL parser — analytics owns query quality.
func CheckSingleSelect(sqlText string) error {
	trimmed := strings.TrimSpace(sqlText)
	if trimmed == "" {
		return fmt.Errorf("sqlText is required")
	}
	if len(trimmed) > 512*1024 {
		return fmt.Errorf("sqlText exceeds maximum length")
	}
	// Reject multi-statement separators outside strings (best-effort).
	if strings.Count(stripSQLLineComments(trimmed), ";") > 0 {
		body := strings.TrimRight(stripSQLLineComments(trimmed), " \t\r\n;")
		if strings.Contains(body, ";") {
			return fmt.Errorf("multi-statement SQL is not allowed")
		}
	}
	upper := strings.ToUpper(strings.TrimLeft(trimmed, " \t\r\n("))
	if !(strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")) {
		return fmt.Errorf("sqlText must be a single SELECT or WITH … SELECT")
	}
	forbidden := []string{
		" INSERT ", " UPDATE ", " DELETE ", " DROP ", " ALTER ", " CREATE ",
		" TRUNCATE ", " GRANT ", " REVOKE ", " EXEC ", " EXECUTE ",
		" INTO OUTFILE", " INTO DUMPFILE", " xp_", " sp_",
	}
	padded := " " + strings.ToUpper(trimmed) + " "
	for _, f := range forbidden {
		if strings.Contains(padded, f) {
			return fmt.Errorf("sqlText contains forbidden keyword or construct")
		}
	}
	return nil
}

func stripSQLLineComments(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
