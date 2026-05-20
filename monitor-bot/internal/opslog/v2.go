package opslog

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

type ParseKind string

const (
	ParsedV2       ParseKind = "V2"
	UnparsedText   ParseKind = "UNPARSED_TEXT"
	UnparsedJSON   ParseKind = "UNPARSED_JSON"
	SeverityLow    string    = "LOW"
	SeverityMed    string    = "MEDIUM"
	SeverityHigh   string    = "HIGH"
	SeverityCrit   string    = "CRITICAL"
	SeverityUnk    string    = "UNKNOWN"
	UnknownErrCode           = "UNKNOWN_ERROR_CODE"
)

type V2OpsLog struct {
	Timestamp string      `json:"timestamp"`
	Level     string      `json:"level,omitempty"`
	LogType   string      `json:"logType"`
	Message   string      `json:"message,omitempty"`
	Service   V2Service   `json:"service"`
	Trace     *V2Trace    `json:"trace,omitempty"`
	HTTP      *V2HTTP     `json:"http,omitempty"`
	Response  *V2Response `json:"response,omitempty"`
	Tags      []string    `json:"tags,omitempty"`
}

type V2Service struct {
	Name       string `json:"name"`
	Domain     string `json:"domain,omitempty"`
	DomainCode int    `json:"domainCode,omitempty"`
	Version    string `json:"version,omitempty"`
	InstanceID string `json:"instanceId,omitempty"`
}

type V2Trace struct {
	TraceID   string `json:"traceId,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

type V2HTTP struct {
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	Route      string `json:"route,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	LatencyMs  int    `json:"latencyMs,omitempty"`
}

type V2Response struct {
	Success any      `json:"success,omitempty"`
	Error   *V2Error `json:"error,omitempty"`
}

type V2Error struct {
	Code    int    `json:"code,omitempty"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message,omitempty"`
	Alert   string `json:"alert,omitempty"`
}

type ParseResult struct {
	Kind ParseKind
	Log  *V2OpsLog
}

type ErrorCodeInfo struct {
	Valid        bool
	ServiceCode  int
	ServiceName  string
	CategoryCode int
	CategoryName string
	DetailCode   int
}

type AlertDecision struct {
	Alert         bool
	Mention       bool
	AggregateOnly bool
	Severity      string
	Reason        string
	ErrorCode     ErrorCodeInfo
	Domain        string
}

var serviceCodeNames = map[int]string{
	1: "gateway",
	2: "auth",
	3: "user",
	4: "report",
	5: "judge",
	6: "blog",
	9: "common",
}

var categoryCodeNames = map[int]string{
	0: "general",
	1: "auth",
	2: "permission",
	3: "validation",
	4: "business",
	5: "not_found",
	6: "conflict",
	7: "external_system",
	8: "internal_error",
}

var errorCodeOverrides = map[int]ErrorCodeInfo{
	60701: overrideErrorCode(6, 7, 1),
	90701: overrideErrorCode(9, 7, 1),
	90801: overrideErrorCode(9, 8, 1),
}

var explicitCriticalCodes = map[int]struct{}{
	18801: {},
	21801: {},
	28101: {},
	38101: {},
	38801: {},
	48801: {},
	54801: {},
	68801: {},
	90801: {},
	98801: {},
}

var knownHighCodes = map[int]struct{}{
	21701: {},
	34801: {},
	34802: {},
	44701: {},
	44801: {},
	44802: {},
	44803: {},
	44804: {},
	44805: {},
	44806: {},
	44807: {},
	54701: {},
	54702: {},
	54703: {},
	54802: {},
	54803: {},
	54804: {},
	54805: {},
	60701: {},
	64801: {},
	64802: {},
	64803: {},
	64804: {},
	64805: {},
	90701: {},
}

func ParseV2Log(rawMessage string) ParseResult {
	rawMessage = strings.TrimSpace(rawMessage)
	if rawMessage == "" {
		return ParseResult{Kind: UnparsedText}
	}
	var log V2OpsLog
	if err := json.Unmarshal([]byte(rawMessage), &log); err != nil {
		return ParseResult{Kind: UnparsedText}
	}
	if !isV2Shape(log) {
		return ParseResult{Kind: UnparsedJSON}
	}
	return ParseResult{Kind: ParsedV2, Log: &log}
}

func isV2Shape(log V2OpsLog) bool {
	if strings.TrimSpace(log.LogType) == "" {
		return false
	}
	if strings.TrimSpace(log.Service.Name) == "" && strings.TrimSpace(log.Service.Domain) == "" {
		return false
	}
	return log.Trace != nil || log.HTTP != nil || log.Response != nil
}

func ParseErrorCode(code int) ErrorCodeInfo {
	if info, ok := errorCodeOverrides[code]; ok {
		return info
	}
	if code < 10000 || code > 99999 {
		return ErrorCodeInfo{ServiceName: UnknownErrCode, CategoryName: UnknownErrCode}
	}
	serviceCode := code / 10000
	categoryCode := (code / 1000) % 10
	detailCode := code % 1000
	if serviceCode == 2 && categoryCode == 1 {
		authCategoryCode := (code / 100) % 10
		if authCategoryCode >= 1 && authCategoryCode <= 8 {
			categoryCode = authCategoryCode
			detailCode = code % 100
		}
	}
	serviceName, serviceOK := serviceCodeNames[serviceCode]
	categoryName, categoryOK := categoryCodeNames[categoryCode]
	if !serviceOK || !categoryOK {
		return ErrorCodeInfo{
			ServiceCode: serviceCode, CategoryCode: categoryCode, DetailCode: detailCode,
			ServiceName: UnknownErrCode, CategoryName: UnknownErrCode,
		}
	}
	return ErrorCodeInfo{
		Valid:        true,
		ServiceCode:  serviceCode,
		ServiceName:  serviceName,
		CategoryCode: categoryCode,
		CategoryName: categoryName,
		DetailCode:   detailCode,
	}
}

func ParseErrorCodeString(value string) ErrorCodeInfo {
	code, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return ErrorCodeInfo{ServiceName: UnknownErrCode, CategoryName: UnknownErrCode}
	}
	return ParseErrorCode(code)
}

func ResolveServiceDomain(log V2OpsLog) string {
	if domain := strings.ToLower(strings.TrimSpace(log.Service.Domain)); domain != "" {
		return domain
	}
	if domain := domainFromCode(log.Service.DomainCode); domain != "" {
		return domain
	}
	if name := strings.ToLower(strings.TrimSpace(log.Service.Name)); name != "" {
		return normalizeServiceName(name)
	}
	return "unknown"
}

func ResolveServiceDomainFromFields(row map[string]string) string {
	if domain := strings.ToLower(strings.TrimSpace(row["service.domain"])); domain != "" {
		return domain
	}
	if domain := domainFromCode(parseInt(row["service.domainCode"])); domain != "" {
		return domain
	}
	if name := strings.ToLower(strings.TrimSpace(row["service.name"])); name != "" {
		return normalizeServiceName(name)
	}
	return "unknown"
}

func DecideV2Alert(log V2OpsLog) AlertDecision {
	info := ErrorCodeInfo{ServiceName: UnknownErrCode, CategoryName: UnknownErrCode}
	code := 0
	if log.Response != nil && log.Response.Error != nil && log.Response.Error.Code != 0 {
		code = log.Response.Error.Code
		info = ParseErrorCode(code)
	}
	logType := strings.ToUpper(strings.TrimSpace(log.LogType))
	severity := ResolveV2Severity(log, info)
	explicitCritical := severity == SeverityCrit
	_, highCode := knownHighCodes[code]
	externalSystem := info.Valid && info.CategoryCode == 7
	httpServerError := log.HTTP != nil && log.HTTP.StatusCode >= 500
	alert := explicitCritical ||
		logType == "EVENT_ERROR" ||
		(logType == "API_ERROR" && httpServerError) ||
		externalSystem ||
		highCode
	decision := AlertDecision{
		Alert:         alert,
		Mention:       alert && severity == SeverityCrit,
		AggregateOnly: !alert,
		Severity:      severity,
		ErrorCode:     info,
		Domain:        ResolveServiceDomain(log),
	}
	switch {
	case explicitCritical:
		decision.Reason = "critical"
	case externalSystem:
		decision.Reason = "external_system"
	case highCode:
		decision.Reason = "operation_failure"
	case logType == "EVENT_ERROR":
		decision.Reason = "EVENT_ERROR"
	case httpServerError:
		decision.Reason = "http_5xx"
	default:
		decision.Reason = "aggregate_only"
	}
	return decision
}

func ResolveV2Severity(log V2OpsLog, info ErrorCodeInfo) string {
	code := 0
	if log.Response != nil && log.Response.Error != nil {
		code = log.Response.Error.Code
	}
	logType := strings.ToUpper(strings.TrimSpace(log.LogType))
	switch {
	case strings.EqualFold(strings.TrimSpace(log.Level), SeverityCrit):
		return SeverityCrit
	case hasCode(explicitCriticalCodes, code):
		return SeverityCrit
	case info.Valid && info.CategoryCode == 7:
		return SeverityHigh
	case hasCode(knownHighCodes, code):
		return SeverityHigh
	case logType == "EVENT_ERROR":
		return SeverityHigh
	case logType == "API_ERROR" && log.HTTP != nil && log.HTTP.StatusCode >= 500:
		return SeverityHigh
	case logType == "API_SLOW":
		return SeverityMed
	case logType == "SECURITY":
		return SeverityMed
	case info.Valid && info.CategoryCode >= 1 && info.CategoryCode <= 6:
		return SeverityLow
	default:
		return SeverityUnk
	}
}

func RowToV2OpsLog(row map[string]string) V2OpsLog {
	var errInfo *V2Error
	if code := parseInt(row["response.error.code"]); code != 0 ||
		strings.TrimSpace(row["response.error.value"]) != "" ||
		strings.TrimSpace(row["response.error.message"]) != "" ||
		strings.TrimSpace(row["response.error.alert"]) != "" {
		errInfo = &V2Error{
			Code:    code,
			Value:   row["response.error.value"],
			Message: row["response.error.message"],
			Alert:   row["response.error.alert"],
		}
	}
	return V2OpsLog{
		Timestamp: firstNonEmpty(row["@timestamp"], row["timestamp"]),
		Level:     row["level"],
		LogType:   row["logType"],
		Message:   row["message"],
		Service: V2Service{
			Name:       row["service.name"],
			Domain:     row["service.domain"],
			DomainCode: parseInt(row["service.domainCode"]),
			Version:    row["service.version"],
			InstanceID: row["service.instanceId"],
		},
		Trace: &V2Trace{
			TraceID:   row["trace.traceId"],
			RequestID: row["trace.requestId"],
		},
		HTTP: &V2HTTP{
			Method:     row["http.method"],
			Path:       row["http.path"],
			Route:      row["http.route"],
			StatusCode: parseInt(row["http.statusCode"]),
			LatencyMs:  parseInt(row["http.latencyMs"]),
		},
		Response: &V2Response{
			Success: row["response.success"],
			Error:   errInfo,
		},
	}
}

func DecideV2AlertFromFields(row map[string]string) AlertDecision {
	return DecideV2Alert(RowToV2OpsLog(row))
}

func FormatV2Alert(log V2OpsLog, decision AlertDecision, mention string) string {
	var b strings.Builder
	if decision.Mention {
		b.WriteString(mention)
	}
	domain := firstNonEmpty(decision.Domain, ResolveServiceDomain(log))
	fmt.Fprintf(&b, "🚨 %s | %s\n\n", security.SanitizeText(log.LogType), security.SanitizeText(domain))
	fmt.Fprintf(&b, "Service   %s\n", security.SanitizeText(domain))
	if log.Response != nil && log.Response.Error != nil && log.Response.Error.Code != 0 {
		fmt.Fprintf(&b, "Code      %d\n", log.Response.Error.Code)
	} else {
		b.WriteString("Code      -\n")
	}
	categoryService := decision.ErrorCode.ServiceName
	categoryName := decision.ErrorCode.CategoryName
	if !decision.ErrorCode.Valid {
		categoryService = UnknownErrCode
		categoryName = UnknownErrCode
	}
	fmt.Fprintf(&b, "Category  %s / %s\n", security.SanitizeText(categoryService), security.SanitizeText(categoryName))
	if log.Response != nil && log.Response.Error != nil {
		if strings.TrimSpace(log.Response.Error.Value) != "" {
			fmt.Fprintf(&b, "Value     %s\n", security.SanitizeText(log.Response.Error.Value))
		}
	}
	if log.HTTP != nil {
		path := firstNonEmpty(log.HTTP.Route, log.HTTP.Path, "-")
		fmt.Fprintf(&b, "HTTP      %s %s -> %s\n", security.SanitizeText(firstNonEmpty(log.HTTP.Method, "-")), security.SanitizeText(path), statusValue(log.HTTP.StatusCode))
		if log.HTTP.LatencyMs > 0 {
			fmt.Fprintf(&b, "Latency   %dms\n", log.HTTP.LatencyMs)
		}
	}
	if log.Trace != nil && strings.TrimSpace(log.Trace.TraceID) != "" {
		fmt.Fprintf(&b, "Trace     %s\n", security.SanitizeText(log.Trace.TraceID))
	}
	if strings.TrimSpace(log.Message) != "" {
		fmt.Fprintf(&b, "\nMessage   %s\n", security.SanitizeText(log.Message))
	}
	if log.Response != nil && log.Response.Error != nil && strings.TrimSpace(log.Response.Error.Alert) != "" {
		fmt.Fprintf(&b, "Alert     %s\n", security.SanitizeText(log.Response.Error.Alert))
	}
	b.WriteString("\nNext\n")
	if log.Trace != nil && strings.TrimSpace(log.Trace.TraceID) != "" {
		fmt.Fprintf(&b, "/ops logs mode:trace query:%s\n", security.SanitizeText(log.Trace.TraceID))
	}
	fmt.Fprintf(&b, "/ops logs service:%s mode:errors since:30m limit:10", security.SanitizeText(domain))
	return security.SanitizeText(b.String())
}

func domainFromCode(code int) string {
	if code == 0 {
		return ""
	}
	return serviceCodeNames[code]
}

func overrideErrorCode(serviceCode, categoryCode, detailCode int) ErrorCodeInfo {
	return ErrorCodeInfo{
		Valid:        true,
		ServiceCode:  serviceCode,
		ServiceName:  serviceCodeNames[serviceCode],
		CategoryCode: categoryCode,
		CategoryName: categoryCodeNames[categoryCode],
		DetailCode:   detailCode,
	}
}

func hasCode(codes map[int]struct{}, code int) bool {
	_, ok := codes[code]
	return ok
}

func normalizeServiceName(name string) string {
	switch name {
	case "auth", "auth-service":
		return "auth"
	case "web-service", "report-service":
		return "report"
	case "online-judge", "online-judge-service", "judge-service":
		return "judge"
	case "post", "post-service", "blog-service":
		return "blog"
	default:
		return name
	}
}

func parseInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func statusValue(status int) string {
	if status == 0 {
		return "-"
	}
	return strconv.Itoa(status)
}
