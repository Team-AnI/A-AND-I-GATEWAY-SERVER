package security

import (
	"regexp"
	"sort"
	"strings"
)

var allowedOutputFields = map[string]struct{}{
	"@timestamp":             {},
	"env":                    {},
	"service.name":           {},
	"service.domain":         {},
	"service.domainCode":     {},
	"service.version":        {},
	"service.instanceId":     {},
	"level":                  {},
	"logType":                {},
	"trace.traceId":          {},
	"trace.requestId":        {},
	"http.method":            {},
	"http.path":              {},
	"http.route":             {},
	"http.statusCode":        {},
	"http.latencyMs":         {},
	"actor.userId":           {},
	"actor.role":             {},
	"actor.isAuthenticated":  {},
	"response.success":       {},
	"response.error.code":    {},
	"response.error.value":   {},
	"response.error.message": {},
	"response.error.alert":   {},
	"message":                {},
	"tags":                   {},
	"count":                  {},
}

var forbiddenFieldFragments = []string{
	"password",
	"passwordconfirm",
	"accesstoken",
	"refreshtoken",
	"token",
	"authorization",
	"authenticate",
	"headers.authenticate",
	"headers.salt",
	"salt",
	"secret",
	"credential",
	"credentials",
	"cookie",
	"session",
	"privatetestcases",
	"hiddentestcases",
	"expectedoutput",
	"input",
	"output",
	"usercode",
	"sourcecode",
	"code",
	"response.data",
	"request.body",
}

var sensitiveKeyPattern = regexp.MustCompile(`(?i)("?(password|passwordConfirm|accessToken|refreshToken|authorization|authenticate|salt|secret|credentials?|cookie|session|privateTestCases|hiddenTestCases|expectedOutput|userCode|sourceCode|response\.data|request\.body|token|code)"?\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,}]+)`)

func AllowedOutputFields() []string {
	fields := make([]string, 0, len(allowedOutputFields))
	for field := range allowedOutputFields {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func IsForbiddenField(field string) bool {
	normalized := strings.ToLower(strings.TrimSpace(field))
	for _, fragment := range forbiddenFieldFragments {
		if normalized == fragment || strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func SanitizeText(value string) string {
	if value == "" {
		return ""
	}
	value = regexp.MustCompile(`(?i)authorization:\s*bearer\s+[^\s,}]+`).ReplaceAllString(value, "authorization: [REDACTED]")
	value = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/-]+=*`).ReplaceAllString(value, "Bearer [REDACTED]")
	value = regexp.MustCompile(`(?i)authorization:\s*[^\s,}]+`).ReplaceAllString(value, "authorization: [REDACTED]")
	value = sensitiveKeyPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	return value
}

func SanitizeFieldMap(input map[string]string) map[string]string {
	output := make(map[string]string)
	for key, value := range input {
		if _, ok := allowedOutputFields[key]; !ok {
			continue
		}
		output[key] = SanitizeText(value)
	}
	return output
}

func SanitizeObject(input map[string]any) map[string]any {
	return sanitizeObjectWithPrefix("", input)
}

func sanitizeObjectWithPrefix(prefix string, input map[string]any) map[string]any {
	output := make(map[string]any)
	for key, value := range input {
		field := key
		if prefix != "" {
			field = prefix + "." + key
		}
		if IsForbiddenField(field) || IsForbiddenField(key) {
			output[key] = "[REDACTED]"
			continue
		}
		output[key] = sanitizeAny(field, value)
	}
	return output
}

func sanitizeAny(prefix string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeObjectWithPrefix(prefix, typed)
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, sanitizeAny(prefix, item))
		}
		return result
	case string:
		return SanitizeText(typed)
	default:
		return typed
	}
}

func FilterDisplayPairs(input map[string]string) [][2]string {
	sanitized := SanitizeFieldMap(input)
	fields := AllowedOutputFields()
	pairs := make([][2]string, 0, len(sanitized))
	for _, field := range fields {
		value := strings.TrimSpace(sanitized[field])
		if value != "" {
			pairs = append(pairs, [2]string{field, value})
		}
	}
	return pairs
}
