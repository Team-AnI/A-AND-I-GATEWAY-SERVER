package security

import (
	"fmt"
	"strings"
	"testing"
)

func TestSanitizeTextMasksSensitiveValues(t *testing.T) {
	input := `accessToken="abc" refreshToken=def Authorization: Bearer token password=secret`
	got := SanitizeText(input)
	for _, forbidden := range []string{"abc", "def", "Bearer token", "secret"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized text still contains %q: %s", forbidden, got)
		}
	}
}

func TestSanitizeFieldMapDropsForbiddenRawFields(t *testing.T) {
	row := map[string]string{
		"@timestamp":           "2026-05-11T00:00:00Z",
		"request.body":         `{"password":"secret"}`,
		"response.data":        `{"accessToken":"abc"}`,
		"userCode":             "println(1)",
		"sourceCode":           "fun main()",
		"code":                 "raw-code",
		"privateTestCases":     "private",
		"hiddenTestCases":      "hidden",
		"expectedOutput":       "42",
		"headers.salt":         "salt",
		"headers.Authenticate": "auth",
		"message":              `failed password=secret accessToken=abc`,
	}
	got := SanitizeFieldMap(row)
	for _, field := range []string{"request.body", "response.data", "userCode", "sourceCode", "code", "privateTestCases", "hiddenTestCases", "expectedOutput", "headers.salt", "headers.Authenticate"} {
		if _, ok := got[field]; ok {
			t.Fatalf("forbidden field %s should not be present", field)
		}
	}
	if strings.Contains(got["message"], "secret") || strings.Contains(got["message"], "abc") {
		t.Fatalf("message was not sanitized: %s", got["message"])
	}
}

func TestSanitizeObjectRedactsNestedForbiddenFields(t *testing.T) {
	got := SanitizeObject(map[string]any{
		"request": map[string]any{
			"body": map[string]any{"password": "secret"},
		},
		"response": map[string]any{
			"data": map[string]any{"accessToken": "abc"},
		},
		"headers": map[string]any{
			"Authenticate": "auth-value",
			"salt":         "salt-value",
		},
	})
	if strings.Contains(fmt.Sprint(got), "secret") ||
		strings.Contains(fmt.Sprint(got), "abc") ||
		strings.Contains(fmt.Sprint(got), "auth-value") ||
		strings.Contains(fmt.Sprint(got), "salt-value") {
		t.Fatalf("nested sensitive values leaked: %#v", got)
	}
}
