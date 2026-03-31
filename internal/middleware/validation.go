package middleware

import (
	"regexp"
	"strings"
)

var (
	modelNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_\-./:]+$`)
	sanitizPattern   = regexp.MustCompile(`[\x00-\x1f\x7f]`)
)

func ValidateModelName(model string) bool {
	if model == "" || len(model) > 128 {
		return false
	}
	if strings.Contains(model, "..") {
		return false
	}
	if sanitizPattern.MatchString(model) {
		return false
	}
	return modelNamePattern.MatchString(model)
}
