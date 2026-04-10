// Package db provides structured filter expression support for host queries.
// This file implements the FilterExpr type and its SQL translation logic.
package db

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FilterExpr is a discriminated union representing a structured filter expression.
// Groups have Op+Conditions set; leaves have Field+Cmp+Value set.
type FilterExpr struct {
	Op         string       `json:"op,omitempty"`         // "AND" or "OR" (groups only)
	Conditions []FilterExpr `json:"conditions,omitempty"` // sub-expressions (groups only)
	Field      string       `json:"field,omitempty"`      // field name (leaves only)
	Cmp        string       `json:"cmp,omitempty"`        // "is","is_not","contains","gt","lt","between"
	Value      string       `json:"value,omitempty"`      // primary comparison value
	Value2     string       `json:"value2,omitempty"`     // secondary value used by "between"
}

// filterFieldSQL maps FilterExpr field names to their SQL column expressions.
var filterFieldSQL = map[string]string{
	"status":           "h.status",
	"os_family":        "h.os_family",
	"vendor":           "h.vendor",
	"hostname":         "h.hostname",
	"response_time_ms": "h.response_time_ms",
	"first_seen":       "h.first_seen",
	"last_seen":        "h.last_seen",
}

// filterTextFields are simple-column fields that support the "contains" (ILIKE) operator.
var filterTextFields = map[string]bool{
	"status":    true,
	"os_family": true,
	"vendor":    true,
	"hostname":  true,
}

// filterNumericFields are simple-column fields whose values must be parsed as integers.
var filterNumericFields = map[string]bool{
	"response_time_ms": true,
}

// filterDateFields are simple-column fields whose values must be parsed as date/time.
var filterDateFields = map[string]bool{
	"first_seen": true,
	"last_seen":  true,
}

// Repeated string literals extracted as constants to satisfy goconst.
const (
	filterCmpIs          = "is"
	filterCmpIsNot       = "is_not"
	filterCmpContains    = "contains"
	filterCmpGt          = "gt"
	filterCmpLt          = "lt"
	filterCmpBetween     = "between"
	filterFieldOpenPort  = "open_port"
	filterFieldScanCount = "scan_count"
)

const (
	filterFieldTags  = "tags"
	filterFieldGroup = "group"
)

// scanCountSubquery is the correlated subquery used for the "scan_count" aggregate field.
// Split across multiple string constants to stay within the 120-character line limit.
const scanCountSubquery = "(SELECT COUNT(DISTINCT sj_f.id)" +
	" FROM scan_jobs sj_f" +
	" JOIN port_scans ps_f ON ps_f.job_id = sj_f.id" +
	" WHERE ps_f.host_id = h.id AND sj_f.status = 'completed')"

const (
	// maxFilterDepth is the maximum allowed group nesting depth (0-indexed).
	maxFilterDepth = 3

	// maxFilterConditions is the maximum number of conditions allowed in a single group.
	maxFilterConditions = 20
)

// ParseFilterExpr decodes a JSON-encoded filter expression and validates it.
// Returns an error if the JSON is malformed or the expression is structurally invalid.
func ParseFilterExpr(data []byte) (*FilterExpr, error) {
	var expr FilterExpr
	if err := json.Unmarshal(data, &expr); err != nil {
		return nil, fmt.Errorf("invalid filter expression JSON: %w", err)
	}
	if err := validateFilterExpr(&expr, 0); err != nil {
		return nil, err
	}
	return &expr, nil
}

// validateFilterExpr recursively validates a filter expression.
// It enforces depth limits, group structure, and leaf field/operator compatibility.
func validateFilterExpr(expr *FilterExpr, depth int) error {
	if depth > maxFilterDepth {
		return fmt.Errorf("filter expression exceeds maximum nesting depth of %d", maxFilterDepth)
	}

	// ── Group node ──────────────────────────────────────────────────────────
	if expr.Op != "" {
		return validateGroupExpr(expr, depth)
	}

	// ── Leaf node ───────────────────────────────────────────────────────────
	return validateLeafExpr(expr)
}

// validateGroupExpr validates a group (AND/OR) node.
func validateGroupExpr(expr *FilterExpr, depth int) error {
	if expr.Op != "AND" && expr.Op != "OR" {
		return fmt.Errorf("invalid group operator %q: must be AND or OR", expr.Op)
	}
	if len(expr.Conditions) == 0 {
		return fmt.Errorf("group with op %q must have at least one condition", expr.Op)
	}
	if len(expr.Conditions) > maxFilterConditions {
		return fmt.Errorf("group exceeds maximum of %d conditions", maxFilterConditions)
	}
	for i := range expr.Conditions {
		if err := validateFilterExpr(&expr.Conditions[i], depth+1); err != nil {
			return fmt.Errorf("condition[%d]: %w", i, err)
		}
	}
	return nil
}

// validateLeafExpr validates a leaf (field/cmp/value) node.
func validateLeafExpr(expr *FilterExpr) error {
	if expr.Field == "" {
		return fmt.Errorf("filter expression must have either op (group) or field (leaf)")
	}

	_, isSimple := filterFieldSQL[expr.Field]
	isAggregate := expr.Field == filterFieldOpenPort ||
		expr.Field == filterFieldScanCount ||
		expr.Field == filterFieldTags ||
		expr.Field == filterFieldGroup
	if !isSimple && !isAggregate {
		return fmt.Errorf("unknown filter field %q", expr.Field)
	}

	if expr.Value == "" {
		return fmt.Errorf("filter leaf for field %q must have a non-empty value", expr.Field)
	}

	return validateLeafOperator(expr)
}

// validateLeafOperator checks that the operator is valid for the leaf's field type.
func validateLeafOperator(expr *FilterExpr) error {
	// tags field: only supports "contains" and "is_not".
	if expr.Field == filterFieldTags {
		if expr.Cmp != filterCmpContains && expr.Cmp != filterCmpIsNot {
			return fmt.Errorf(
				"operator %q is not valid for %q: use contains or is_not",
				expr.Cmp, filterFieldTags,
			)
		}
		return nil
	}
	// group field: only supports "is" and "is_not".
	if expr.Field == filterFieldGroup {
		if expr.Cmp != filterCmpIs && expr.Cmp != filterCmpIsNot {
			return fmt.Errorf(
				"operator %q is not valid for %q: use is or is_not",
				expr.Cmp, filterFieldGroup,
			)
		}
		return nil
	}
	switch expr.Cmp {
	case filterCmpIs, filterCmpIsNot:
		// Valid for all fields — no additional check needed.
		return nil

	case filterCmpContains:
		if !filterTextFields[expr.Field] {
			return fmt.Errorf(
				"operator %q is only valid for text fields (status, os_family, vendor, hostname), got %q",
				expr.Cmp, expr.Field,
			)
		}
		return nil

	case filterCmpGt, filterCmpLt:
		return validateRangeField(expr.Cmp, expr.Field)

	case filterCmpBetween:
		if err := validateRangeField(expr.Cmp, expr.Field); err != nil {
			return err
		}
		if expr.Value2 == "" {
			return fmt.Errorf("operator %q requires value2 to be set", expr.Cmp)
		}
		return nil

	case "":
		return fmt.Errorf("filter leaf for field %q must have a cmp operator", expr.Field)

	default:
		return fmt.Errorf(
			"unknown cmp operator %q: must be is, is_not, contains, gt, lt, or between",
			expr.Cmp,
		)
	}
}

// validateRangeField checks whether the given field supports range operators (gt/lt/between).
func validateRangeField(cmp, field string) error {
	isRangeable := filterNumericFields[field] || filterDateFields[field] || field == filterFieldScanCount
	if !isRangeable {
		return fmt.Errorf("operator %q is only valid for numeric/date fields, got %q", cmp, field)
	}
	return nil
}

// TranslateFilterExpr converts a validated FilterExpr into a SQL fragment (no WHERE keyword)
// using positional placeholders starting at startIdx.
func TranslateFilterExpr(
	expr *FilterExpr,
	startIdx int,
) (sqlFrag string, args []interface{}, err error) {
	if expr.Op != "" {
		return translateGroupExpr(expr, startIdx)
	}
	return translateLeafExpr(expr, startIdx)
}

// translateGroupExpr handles AND / OR group nodes.
func translateGroupExpr(
	expr *FilterExpr,
	startIdx int,
) (sqlFrag string, args []interface{}, err error) {
	var parts []string
	var allArgs []interface{}
	idx := startIdx

	for i := range expr.Conditions {
		part, a, e := TranslateFilterExpr(&expr.Conditions[i], idx)
		if e != nil {
			return "", nil, e
		}
		parts = append(parts, "("+part+")")
		allArgs = append(allArgs, a...)
		idx += len(a)
	}

	sep := " " + expr.Op + " "
	return strings.Join(parts, sep), allArgs, nil
}

// translateLeafExpr dispatches leaf translation based on field name.
func translateLeafExpr(
	expr *FilterExpr,
	startIdx int,
) (sqlFrag string, args []interface{}, err error) {
	switch expr.Field {
	case filterFieldOpenPort:
		return translateOpenPortExpr(expr, startIdx)
	case filterFieldScanCount:
		return translateScanCountExpr(expr, startIdx)
	case filterFieldTags:
		return translateTagsExpr(expr, startIdx)
	case filterFieldGroup:
		return translateGroupMembershipExpr(expr, startIdx)
	default:
		return translateSimpleExpr(expr, startIdx)
	}
}

// translateTagsExpr handles the tags array-containment field.
// "contains" checks that the host has the given tag; "is_not" inverts it.
func translateTagsExpr(
	expr *FilterExpr,
	startIdx int,
) (sqlFrag string, args []interface{}, err error) {
	subquery := fmt.Sprintf("h.tags @> ARRAY[$%d]::text[]", startIdx)
	if expr.Cmp == filterCmpIsNot {
		subquery = "NOT (" + subquery + ")"
	}
	return subquery, []interface{}{expr.Value}, nil
}

// translateGroupMembershipExpr handles the group membership field via a correlated EXISTS subquery.
// "is" checks that the host is a member of the given group; "is_not" inverts it.
func translateGroupMembershipExpr(
	expr *FilterExpr,
	startIdx int,
) (sqlFrag string, args []interface{}, err error) {
	subquery := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM host_group_members hgm_f"+
			" WHERE hgm_f.host_id = h.id AND hgm_f.group_id = $%d::uuid)",
		startIdx,
	)
	if expr.Cmp == filterCmpIsNot {
		subquery = "NOT " + subquery
	}
	return subquery, []interface{}{expr.Value}, nil
}

// translateOpenPortExpr handles the open_port aggregate field via a correlated EXISTS subquery.
// Operator "is" checks that the port is open; "is_not" wraps with NOT EXISTS.
func translateOpenPortExpr(
	expr *FilterExpr,
	startIdx int,
) (sqlFrag string, args []interface{}, err error) {
	port, e := strconv.Atoi(expr.Value)
	if e != nil || port < 1 || port > 65535 {
		return "", nil, fmt.Errorf(
			"open_port value must be a valid port number (1-65535), got %q", expr.Value,
		)
	}

	subquery := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM port_scans ps_f"+
			" WHERE ps_f.host_id = h.id AND ps_f.port = $%d AND ps_f.state = 'open')",
		startIdx,
	)
	if expr.Cmp == filterCmpIsNot {
		subquery = "NOT " + subquery
	}
	return subquery, []interface{}{port}, nil
}

// translateScanCountExpr handles the scan_count aggregate field via a correlated COUNT subquery.
func translateScanCountExpr(
	expr *FilterExpr,
	startIdx int,
) (sqlFrag string, args []interface{}, err error) {
	switch expr.Cmp {
	case filterCmpIs:
		val, e := strconv.Atoi(expr.Value)
		if e != nil {
			return "", nil, fmt.Errorf("scan_count value must be an integer: %w", e)
		}
		return fmt.Sprintf("%s = $%d", scanCountSubquery, startIdx), []interface{}{val}, nil

	case filterCmpIsNot:
		val, e := strconv.Atoi(expr.Value)
		if e != nil {
			return "", nil, fmt.Errorf("scan_count value must be an integer: %w", e)
		}
		return fmt.Sprintf("%s != $%d", scanCountSubquery, startIdx), []interface{}{val}, nil

	case filterCmpGt:
		val, e := strconv.Atoi(expr.Value)
		if e != nil {
			return "", nil, fmt.Errorf("scan_count value must be an integer: %w", e)
		}
		return fmt.Sprintf("%s > $%d", scanCountSubquery, startIdx), []interface{}{val}, nil

	case filterCmpLt:
		val, e := strconv.Atoi(expr.Value)
		if e != nil {
			return "", nil, fmt.Errorf("scan_count value must be an integer: %w", e)
		}
		return fmt.Sprintf("%s < $%d", scanCountSubquery, startIdx), []interface{}{val}, nil

	case filterCmpBetween:
		val1, e := strconv.Atoi(expr.Value)
		if e != nil {
			return "", nil, fmt.Errorf("scan_count value must be an integer: %w", e)
		}
		val2, e := strconv.Atoi(expr.Value2)
		if e != nil {
			return "", nil, fmt.Errorf("scan_count value2 must be an integer: %w", e)
		}
		sql := fmt.Sprintf("%s BETWEEN $%d AND $%d", scanCountSubquery, startIdx, startIdx+1)
		return sql, []interface{}{val1, val2}, nil

	default:
		return "", nil, fmt.Errorf("unsupported operator %q for scan_count", expr.Cmp)
	}
}

// translateSimpleExpr handles fields that map directly to a SQL column expression.
func translateSimpleExpr(
	expr *FilterExpr,
	startIdx int,
) (sqlFrag string, args []interface{}, err error) {
	col, ok := filterFieldSQL[expr.Field]
	if !ok {
		return "", nil, fmt.Errorf("unknown filter field %q", expr.Field)
	}

	switch expr.Cmp {
	case filterCmpIs:
		val, e := parseFieldValue(expr.Field, expr.Value)
		if e != nil {
			return "", nil, e
		}
		return fmt.Sprintf("%s = $%d", col, startIdx), []interface{}{val}, nil

	case filterCmpIsNot:
		val, e := parseFieldValue(expr.Field, expr.Value)
		if e != nil {
			return "", nil, e
		}
		return fmt.Sprintf("%s != $%d", col, startIdx), []interface{}{val}, nil

	case filterCmpContains:
		pattern := "%" + expr.Value + "%"
		return fmt.Sprintf("%s ILIKE $%d", col, startIdx), []interface{}{pattern}, nil

	case filterCmpGt:
		val, e := parseFieldValue(expr.Field, expr.Value)
		if e != nil {
			return "", nil, e
		}
		return fmt.Sprintf("%s > $%d", col, startIdx), []interface{}{val}, nil

	case filterCmpLt:
		val, e := parseFieldValue(expr.Field, expr.Value)
		if e != nil {
			return "", nil, e
		}
		return fmt.Sprintf("%s < $%d", col, startIdx), []interface{}{val}, nil

	case filterCmpBetween:
		val1, e := parseFieldValue(expr.Field, expr.Value)
		if e != nil {
			return "", nil, e
		}
		val2, e := parseFieldValue(expr.Field, expr.Value2)
		if e != nil {
			return "", nil, e
		}
		sql := fmt.Sprintf("%s BETWEEN $%d AND $%d", col, startIdx, startIdx+1)
		return sql, []interface{}{val1, val2}, nil

	default:
		return "", nil, fmt.Errorf("unsupported operator %q for field %q", expr.Cmp, expr.Field)
	}
}

// parseFieldValue converts a raw string value to the appropriate Go type for the given field.
// Numeric fields are parsed as int; date fields are parsed as time.Time; others remain strings.
func parseFieldValue(field, value string) (interface{}, error) {
	if filterNumericFields[field] {
		val, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("field %q value %q must be an integer: %w", field, value, err)
		}
		return val, nil
	}
	if filterDateFields[field] {
		return parseDateTimeValue(value)
	}
	return value, nil
}

// parseDateTimeValue parses a date/time string in RFC3339 or date-only ("2006-01-02") format.
func parseDateTimeValue(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf(
		"invalid date/time value %q: must be RFC3339 or YYYY-MM-DD format", s,
	)
}
