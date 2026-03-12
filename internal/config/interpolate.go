package config

import (
	"os"
	"regexp"
)

// envVarPattern matches ${VAR_NAME} placeholders anywhere in a byte slice.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Interpolate replaces every ${VAR_NAME} occurrence in data with the value of
// the corresponding environment variable. If the variable is not set, it is
// replaced with an empty string. The input slice is not modified; a new slice
// is returned.
func Interpolate(data []byte) []byte {
	return envVarPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		// FindSubmatch on a match that was produced by the same pattern is
		// guaranteed to return at least two submatches; sub[1] is the name.
		sub := envVarPattern.FindSubmatch(match)
		return []byte(os.Getenv(string(sub[1])))
	})
}
