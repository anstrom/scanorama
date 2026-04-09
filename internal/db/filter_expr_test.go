// Package db provides unit tests for structured filter expression parsing and SQL translation.
package db

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── ParseFilterExpr ────────────────────────────────────────────────────────────

func TestFilterExprParse(t *testing.T) {
	t.Run("valid_group_and", func(t *testing.T) {
		data := []byte(`{"op":"AND","conditions":[{"field":"status","cmp":"is","value":"up"}]}`)
		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
		assert.Equal(t, "AND", expr.Op)
		require.Len(t, expr.Conditions, 1)
		assert.Equal(t, "status", expr.Conditions[0].Field)
		assert.Equal(t, "is", expr.Conditions[0].Cmp)
		assert.Equal(t, "up", expr.Conditions[0].Value)
	})

	t.Run("valid_group_or", func(t *testing.T) {
		data := []byte(`{
			"op":"OR",
			"conditions":[
				{"field":"status","cmp":"is","value":"up"},
				{"field":"status","cmp":"is","value":"down"}
			]
		}`)
		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
		assert.Equal(t, "OR", expr.Op)
		assert.Len(t, expr.Conditions, 2)
	})

	t.Run("valid_leaf_condition", func(t *testing.T) {
		data := []byte(`{"field":"status","cmp":"is","value":"up"}`)
		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
		assert.Equal(t, "status", expr.Field)
		assert.Equal(t, "is", expr.Cmp)
		assert.Equal(t, "up", expr.Value)
		assert.Empty(t, expr.Op)
		assert.Empty(t, expr.Conditions)
	})

	t.Run("valid_leaf_contains", func(t *testing.T) {
		data := []byte(`{"field":"hostname","cmp":"contains","value":"web"}`)
		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
		assert.Equal(t, "contains", expr.Cmp)
	})

	t.Run("valid_leaf_between", func(t *testing.T) {
		data := []byte(`{"field":"response_time_ms","cmp":"between","value":"100","value2":"500"}`)
		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
		assert.Equal(t, "between", expr.Cmp)
		assert.Equal(t, "100", expr.Value)
		assert.Equal(t, "500", expr.Value2)
	})

	t.Run("valid_open_port_is", func(t *testing.T) {
		data := []byte(`{"field":"open_port","cmp":"is","value":"443"}`)
		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
		assert.Equal(t, "open_port", expr.Field)
	})

	t.Run("valid_scan_count_gt", func(t *testing.T) {
		data := []byte(`{"field":"scan_count","cmp":"gt","value":"3"}`)
		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
		assert.Equal(t, "scan_count", expr.Field)
	})

	t.Run("valid_nested_group", func(t *testing.T) {
		data := []byte(`{
			"op":"AND",
			"conditions":[
				{"field":"status","cmp":"is","value":"up"},
				{
					"op":"OR",
					"conditions":[
						{"field":"os_family","cmp":"is","value":"Linux"},
						{"field":"vendor","cmp":"contains","value":"cisco"}
					]
				}
			]
		}`)
		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
		assert.Equal(t, "AND", expr.Op)
		require.Len(t, expr.Conditions, 2)
		assert.Equal(t, "OR", expr.Conditions[1].Op)
	})

	t.Run("invalid_json", func(t *testing.T) {
		data := []byte(`{invalid json}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid filter expression JSON")
	})

	t.Run("invalid_field", func(t *testing.T) {
		data := []byte(`{"field":"unknown_field","cmp":"is","value":"x"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown filter field")
	})

	t.Run("invalid_operator", func(t *testing.T) {
		data := []byte(`{"field":"status","cmp":"badop","value":"x"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown cmp operator")
	})

	t.Run("invalid_group_operator", func(t *testing.T) {
		data := []byte(`{"op":"XOR","conditions":[{"field":"status","cmp":"is","value":"up"}]}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid group operator")
	})

	t.Run("empty_conditions", func(t *testing.T) {
		data := []byte(`{"op":"AND","conditions":[]}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have at least one condition")
	})

	t.Run("missing_cmp", func(t *testing.T) {
		data := []byte(`{"field":"status","value":"up"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cmp")
	})

	t.Run("missing_value", func(t *testing.T) {
		data := []byte(`{"field":"status","cmp":"is"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value")
	})

	t.Run("missing_field_and_op", func(t *testing.T) {
		data := []byte(`{"cmp":"is","value":"up"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "field")
	})

	t.Run("contains_on_non_text_field", func(t *testing.T) {
		data := []byte(`{"field":"response_time_ms","cmp":"contains","value":"100"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only valid for text fields")
	})

	t.Run("gt_on_text_field", func(t *testing.T) {
		data := []byte(`{"field":"status","cmp":"gt","value":"up"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only valid for numeric/date fields")
	})

	t.Run("between_missing_value2", func(t *testing.T) {
		data := []byte(`{"field":"response_time_ms","cmp":"between","value":"100"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires value2")
	})

	t.Run("too_deep_nesting", func(t *testing.T) {
		// Build a structure that exceeds maxFilterDepth (3).
		// root(0) → level1(1) → level2(2) → level3(3) → level4(4) → FAIL (depth 4 > 3)
		leaf := FilterExpr{Field: "status", Cmp: "is", Value: "up"}
		level4 := FilterExpr{Op: "AND", Conditions: []FilterExpr{leaf}}
		level3 := FilterExpr{Op: "AND", Conditions: []FilterExpr{level4}}
		level2 := FilterExpr{Op: "AND", Conditions: []FilterExpr{level3}}
		level1 := FilterExpr{Op: "AND", Conditions: []FilterExpr{level2}}
		root := FilterExpr{Op: "AND", Conditions: []FilterExpr{level1}}

		data, err := json.Marshal(root)
		require.NoError(t, err)

		_, err = ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum nesting depth")
	})

	t.Run("max_depth_is_accepted", func(t *testing.T) {
		// root(0) → level1(1) → level2(2) → leaf(3) — leaf is validated at depth 3.
		// 3 > maxFilterDepth (3) is false, so this must pass.
		leaf := FilterExpr{Field: "status", Cmp: "is", Value: "up"}
		level2 := FilterExpr{Op: "AND", Conditions: []FilterExpr{leaf}}
		level1 := FilterExpr{Op: "AND", Conditions: []FilterExpr{level2}}
		root := FilterExpr{Op: "AND", Conditions: []FilterExpr{level1}}

		data, err := json.Marshal(root)
		require.NoError(t, err)

		expr, err := ParseFilterExpr(data)
		require.NoError(t, err)
		require.NotNil(t, expr)
	})

	t.Run("too_many_conditions", func(t *testing.T) {
		// Build a group with maxFilterConditions+1 children.
		conditions := make([]FilterExpr, maxFilterConditions+1)
		for i := range conditions {
			conditions[i] = FilterExpr{Field: "status", Cmp: "is", Value: "up"}
		}
		root := FilterExpr{Op: "AND", Conditions: conditions}
		data, err := json.Marshal(root)
		require.NoError(t, err)

		_, err = ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum")
	})

	t.Run("open_port_contains_rejected", func(t *testing.T) {
		data := []byte(`{"field":"open_port","cmp":"contains","value":"80"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only valid for text fields")
	})

	t.Run("open_port_gt_rejected", func(t *testing.T) {
		data := []byte(`{"field":"open_port","cmp":"gt","value":"80"}`)
		_, err := ParseFilterExpr(data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only valid for numeric/date fields")
	})
}

// ── TranslateFilterExpr ────────────────────────────────────────────────────────

func TestFilterExprTranslate(t *testing.T) {
	// ── Simple leaf conditions ────────────────────────────────────────────────

	t.Run("simple_is", func(t *testing.T) {
		expr := &FilterExpr{Field: "status", Cmp: "is", Value: "up"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.status = $1", sql)
		assert.Equal(t, []interface{}{"up"}, args)
	})

	t.Run("simple_is_not", func(t *testing.T) {
		expr := &FilterExpr{Field: "os_family", Cmp: "is_not", Value: "Linux"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.os_family != $1", sql)
		assert.Equal(t, []interface{}{"Linux"}, args)
	})

	t.Run("simple_contains", func(t *testing.T) {
		expr := &FilterExpr{Field: "hostname", Cmp: "contains", Value: "server"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.hostname ILIKE $1", sql)
		assert.Equal(t, []interface{}{"%server%"}, args)
	})

	t.Run("simple_contains_vendor", func(t *testing.T) {
		expr := &FilterExpr{Field: "vendor", Cmp: "contains", Value: "cisco"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.vendor ILIKE $1", sql)
		assert.Equal(t, []interface{}{"%cisco%"}, args)
	})

	t.Run("simple_gt_numeric", func(t *testing.T) {
		expr := &FilterExpr{Field: "response_time_ms", Cmp: "gt", Value: "100"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.response_time_ms > $1", sql)
		assert.Equal(t, []interface{}{100}, args)
	})

	t.Run("simple_lt_numeric", func(t *testing.T) {
		expr := &FilterExpr{Field: "response_time_ms", Cmp: "lt", Value: "500"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.response_time_ms < $1", sql)
		assert.Equal(t, []interface{}{500}, args)
	})

	t.Run("simple_between_numeric", func(t *testing.T) {
		expr := &FilterExpr{Field: "response_time_ms", Cmp: "between", Value: "100", Value2: "500"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.response_time_ms BETWEEN $1 AND $2", sql)
		assert.Equal(t, []interface{}{100, 500}, args)
	})

	// ── Date field handling ────────────────────────────────────────────────────

	t.Run("date_field_rfc3339", func(t *testing.T) {
		expr := &FilterExpr{Field: "first_seen", Cmp: "gt", Value: "2024-01-15T10:00:00Z"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.first_seen > $1", sql)
		require.Len(t, args, 1)
		ts, ok := args[0].(time.Time)
		require.True(t, ok, "expected a time.Time argument")
		assert.Equal(t, 2024, ts.Year())
		assert.Equal(t, time.January, ts.Month())
		assert.Equal(t, 15, ts.Day())
	})

	t.Run("date_field_date_only", func(t *testing.T) {
		expr := &FilterExpr{Field: "last_seen", Cmp: "is", Value: "2024-03-20"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.last_seen = $1", sql)
		require.Len(t, args, 1)
		ts, ok := args[0].(time.Time)
		require.True(t, ok, "expected a time.Time argument")
		assert.Equal(t, 2024, ts.Year())
		assert.Equal(t, time.March, ts.Month())
		assert.Equal(t, 20, ts.Day())
	})

	t.Run("date_field_between", func(t *testing.T) {
		expr := &FilterExpr{
			Field:  "first_seen",
			Cmp:    "between",
			Value:  "2024-01-01",
			Value2: "2024-12-31",
		}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "h.first_seen BETWEEN $1 AND $2", sql)
		require.Len(t, args, 2)
		_, ok1 := args[0].(time.Time)
		_, ok2 := args[1].(time.Time)
		assert.True(t, ok1, "expected time.Time for value")
		assert.True(t, ok2, "expected time.Time for value2")
	})

	// ── Group expressions ──────────────────────────────────────────────────────

	t.Run("group_and_two_conditions", func(t *testing.T) {
		expr := &FilterExpr{
			Op: "AND",
			Conditions: []FilterExpr{
				{Field: "status", Cmp: "is", Value: "up"},
				{Field: "vendor", Cmp: "contains", Value: "cisco"},
			},
		}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "(h.status = $1) AND (h.vendor ILIKE $2)", sql)
		assert.Equal(t, []interface{}{"up", "%cisco%"}, args)
	})

	t.Run("group_or_two_conditions", func(t *testing.T) {
		expr := &FilterExpr{
			Op: "OR",
			Conditions: []FilterExpr{
				{Field: "status", Cmp: "is", Value: "up"},
				{Field: "status", Cmp: "is", Value: "down"},
			},
		}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "(h.status = $1) OR (h.status = $2)", sql)
		assert.Equal(t, []interface{}{"up", "down"}, args)
	})

	t.Run("nested_and_or_group", func(t *testing.T) {
		expr := &FilterExpr{
			Op: "AND",
			Conditions: []FilterExpr{
				{Field: "status", Cmp: "is", Value: "up"},
				{
					Op: "OR",
					Conditions: []FilterExpr{
						{Field: "os_family", Cmp: "is", Value: "Linux"},
						{Field: "os_family", Cmp: "is", Value: "Windows"},
					},
				},
			},
		}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		// Outer AND wraps both children in parens.
		assert.True(t, strings.HasPrefix(sql, "(h.status = $1) AND ("), "expected AND structure, got: %s", sql)
		assert.Contains(t, sql, "OR")
		assert.Contains(t, sql, "$2")
		assert.Contains(t, sql, "$3")
		assert.Equal(t, []interface{}{"up", "Linux", "Windows"}, args)
	})

	t.Run("group_three_conditions", func(t *testing.T) {
		expr := &FilterExpr{
			Op: "AND",
			Conditions: []FilterExpr{
				{Field: "status", Cmp: "is", Value: "up"},
				{Field: "os_family", Cmp: "is", Value: "Linux"},
				{Field: "hostname", Cmp: "contains", Value: "web"},
			},
		}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, "(h.status = $1) AND (h.os_family = $2) AND (h.hostname ILIKE $3)", sql)
		assert.Equal(t, []interface{}{"up", "Linux", "%web%"}, args)
	})

	// ── open_port aggregate ────────────────────────────────────────────────────

	t.Run("open_port_is", func(t *testing.T) {
		expr := &FilterExpr{Field: "open_port", Cmp: "is", Value: "80"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Contains(t, sql, "EXISTS")
		assert.NotContains(t, sql, "NOT EXISTS")
		assert.Contains(t, sql, "ps_f.port = $1")
		assert.Contains(t, sql, "ps_f.state = 'open'")
		assert.Equal(t, []interface{}{80}, args)
	})

	t.Run("open_port_is_not", func(t *testing.T) {
		expr := &FilterExpr{Field: "open_port", Cmp: "is_not", Value: "443"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(sql, "NOT EXISTS"), "expected NOT EXISTS prefix, got: %s", sql)
		assert.Contains(t, sql, "ps_f.port = $1")
		assert.Equal(t, []interface{}{443}, args)
	})

	t.Run("open_port_high_port", func(t *testing.T) {
		expr := &FilterExpr{Field: "open_port", Cmp: "is", Value: "65535"}
		_, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Equal(t, []interface{}{65535}, args)
	})

	t.Run("open_port_startIdx_offset", func(t *testing.T) {
		expr := &FilterExpr{Field: "open_port", Cmp: "is", Value: "22"}
		sql, args, err := TranslateFilterExpr(expr, 5)
		require.NoError(t, err)
		assert.Contains(t, sql, "ps_f.port = $5")
		assert.Equal(t, []interface{}{22}, args)
	})

	// ── scan_count aggregate ───────────────────────────────────────────────────

	t.Run("scan_count_is", func(t *testing.T) {
		expr := &FilterExpr{Field: "scan_count", Cmp: "is", Value: "0"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Contains(t, sql, "COUNT(DISTINCT sj_f.id)")
		assert.Contains(t, sql, "= $1")
		assert.Equal(t, []interface{}{0}, args)
	})

	t.Run("scan_count_is_not", func(t *testing.T) {
		expr := &FilterExpr{Field: "scan_count", Cmp: "is_not", Value: "0"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Contains(t, sql, "COUNT(DISTINCT sj_f.id)")
		assert.Contains(t, sql, "!= $1")
		assert.Equal(t, []interface{}{0}, args)
	})

	t.Run("scan_count_gt", func(t *testing.T) {
		expr := &FilterExpr{Field: "scan_count", Cmp: "gt", Value: "5"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Contains(t, sql, "COUNT(DISTINCT sj_f.id)")
		assert.Contains(t, sql, "> $1")
		assert.Equal(t, []interface{}{5}, args)
	})

	t.Run("scan_count_lt", func(t *testing.T) {
		expr := &FilterExpr{Field: "scan_count", Cmp: "lt", Value: "10"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Contains(t, sql, "COUNT(DISTINCT sj_f.id)")
		assert.Contains(t, sql, "< $1")
		assert.Equal(t, []interface{}{10}, args)
	})

	t.Run("scan_count_between", func(t *testing.T) {
		expr := &FilterExpr{Field: "scan_count", Cmp: "between", Value: "3", Value2: "10"}
		sql, args, err := TranslateFilterExpr(expr, 1)
		require.NoError(t, err)
		assert.Contains(t, sql, "COUNT(DISTINCT sj_f.id)")
		assert.Contains(t, sql, "BETWEEN $1 AND $2")
		assert.Equal(t, []interface{}{3, 10}, args)
	})

	// ── Placeholder numbering (startIdx) ──────────────────────────────────────

	t.Run("startIdx_offset_simple", func(t *testing.T) {
		expr := &FilterExpr{Field: "status", Cmp: "is", Value: "up"}
		sql, args, err := TranslateFilterExpr(expr, 5)
		require.NoError(t, err)
		assert.Equal(t, "h.status = $5", sql)
		assert.Equal(t, []interface{}{"up"}, args)
	})

	t.Run("startIdx_offset_group", func(t *testing.T) {
		// With startIdx=3, first condition uses $3, second uses $4.
		expr := &FilterExpr{
			Op: "AND",
			Conditions: []FilterExpr{
				{Field: "status", Cmp: "is", Value: "up"},
				{Field: "vendor", Cmp: "contains", Value: "cisco"},
			},
		}
		sql, args, err := TranslateFilterExpr(expr, 3)
		require.NoError(t, err)
		assert.Equal(t, "(h.status = $3) AND (h.vendor ILIKE $4)", sql)
		assert.Equal(t, []interface{}{"up", "%cisco%"}, args)
	})

	t.Run("startIdx_offset_between", func(t *testing.T) {
		// between uses two consecutive placeholders starting at the given index.
		expr := &FilterExpr{Field: "response_time_ms", Cmp: "between", Value: "10", Value2: "200"}
		sql, args, err := TranslateFilterExpr(expr, 7)
		require.NoError(t, err)
		assert.Equal(t, "h.response_time_ms BETWEEN $7 AND $8", sql)
		assert.Equal(t, []interface{}{10, 200}, args)
	})

	t.Run("startIdx_consecutive_in_group_with_between", func(t *testing.T) {
		// Group: first child uses $2 (between: $2 and $3), second child uses $4.
		expr := &FilterExpr{
			Op: "AND",
			Conditions: []FilterExpr{
				{Field: "response_time_ms", Cmp: "between", Value: "100", Value2: "500"},
				{Field: "status", Cmp: "is", Value: "up"},
			},
		}
		sql, args, err := TranslateFilterExpr(expr, 2)
		require.NoError(t, err)
		assert.Equal(t, "(h.response_time_ms BETWEEN $2 AND $3) AND (h.status = $4)", sql)
		assert.Equal(t, []interface{}{100, 500, "up"}, args)
	})

	t.Run("startIdx_scan_count_between_offset", func(t *testing.T) {
		expr := &FilterExpr{Field: "scan_count", Cmp: "between", Value: "1", Value2: "5"}
		sql, args, err := TranslateFilterExpr(expr, 4)
		require.NoError(t, err)
		assert.Contains(t, sql, "BETWEEN $4 AND $5")
		assert.Equal(t, []interface{}{1, 5}, args)
	})

	// ── Error cases ────────────────────────────────────────────────────────────

	t.Run("error_bad_port_non_numeric", func(t *testing.T) {
		expr := &FilterExpr{Field: "open_port", Cmp: "is", Value: "notaport"}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "valid port number")
	})

	t.Run("error_bad_port_zero", func(t *testing.T) {
		expr := &FilterExpr{Field: "open_port", Cmp: "is", Value: "0"}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "valid port number")
	})

	t.Run("error_bad_port_too_high", func(t *testing.T) {
		expr := &FilterExpr{Field: "open_port", Cmp: "is", Value: "99999"}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "valid port number")
	})

	t.Run("error_bad_integer_response_time", func(t *testing.T) {
		expr := &FilterExpr{Field: "response_time_ms", Cmp: "gt", Value: "notanumber"}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be an integer")
	})

	t.Run("error_bad_integer_scan_count", func(t *testing.T) {
		expr := &FilterExpr{Field: "scan_count", Cmp: "is", Value: "abc"}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be an integer")
	})

	t.Run("error_bad_integer_scan_count_value2", func(t *testing.T) {
		expr := &FilterExpr{Field: "scan_count", Cmp: "between", Value: "1", Value2: "notanumber"}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be an integer")
	})

	t.Run("error_bad_date", func(t *testing.T) {
		expr := &FilterExpr{Field: "first_seen", Cmp: "gt", Value: "not-a-date"}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid date/time value")
	})

	t.Run("error_bad_date_value2_between", func(t *testing.T) {
		expr := &FilterExpr{
			Field:  "last_seen",
			Cmp:    "between",
			Value:  "2024-01-01",
			Value2: "not-a-date",
		}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid date/time value")
	})

	t.Run("error_unknown_field_in_translate", func(t *testing.T) {
		// Bypass validation by calling translateSimpleExpr directly via TranslateFilterExpr
		// with a leaf that has an unknown field name (not reachable via ParseFilterExpr,
		// but TranslateFilterExpr should still handle it gracefully).
		expr := &FilterExpr{Field: "nonexistent", Cmp: "is", Value: "x"}
		_, _, err := TranslateFilterExpr(expr, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown filter field")
	})
}

// ── validateFilterExpr (direct) ────────────────────────────────────────────────

func TestValidateFilterExpr(t *testing.T) {
	t.Run("depth_exactly_at_limit_accepted", func(t *testing.T) {
		// depth=3 is the boundary; depth > 3 fails. Depth 3 is maxFilterDepth, which is valid.
		leaf := FilterExpr{Field: "status", Cmp: "is", Value: "up"}
		err := validateFilterExpr(&leaf, maxFilterDepth)
		assert.NoError(t, err)
	})

	t.Run("depth_one_over_limit_rejected", func(t *testing.T) {
		leaf := FilterExpr{Field: "status", Cmp: "is", Value: "up"}
		err := validateFilterExpr(&leaf, maxFilterDepth+1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum nesting depth")
	})

	t.Run("group_with_exactly_max_conditions_accepted", func(t *testing.T) {
		conditions := make([]FilterExpr, maxFilterConditions)
		for i := range conditions {
			conditions[i] = FilterExpr{Field: "status", Cmp: "is", Value: "up"}
		}
		expr := FilterExpr{Op: "AND", Conditions: conditions}
		err := validateFilterExpr(&expr, 0)
		assert.NoError(t, err)
	})

	t.Run("nested_error_wrapped_with_index", func(t *testing.T) {
		// The second condition is invalid; error message should reference condition[1].
		expr := FilterExpr{
			Op: "AND",
			Conditions: []FilterExpr{
				{Field: "status", Cmp: "is", Value: "up"},
				{Field: "bad_field", Cmp: "is", Value: "x"},
			},
		}
		err := validateFilterExpr(&expr, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "condition[1]")
		assert.Contains(t, err.Error(), "unknown filter field")
	})
}

// ── parseDateTimeValue (direct) ────────────────────────────────────────────────

func TestParseDateTimeValue(t *testing.T) {
	t.Run("valid_rfc3339", func(t *testing.T) {
		ts, err := parseDateTimeValue("2024-06-15T08:30:00Z")
		require.NoError(t, err)
		assert.Equal(t, 2024, ts.Year())
		assert.Equal(t, time.June, ts.Month())
		assert.Equal(t, 15, ts.Day())
	})

	t.Run("valid_rfc3339_with_offset", func(t *testing.T) {
		ts, err := parseDateTimeValue("2024-06-15T08:30:00+02:00")
		require.NoError(t, err)
		assert.Equal(t, 2024, ts.Year())
	})

	t.Run("valid_date_only", func(t *testing.T) {
		ts, err := parseDateTimeValue("2024-01-31")
		require.NoError(t, err)
		assert.Equal(t, 2024, ts.Year())
		assert.Equal(t, time.January, ts.Month())
		assert.Equal(t, 31, ts.Day())
	})

	t.Run("invalid_format", func(t *testing.T) {
		_, err := parseDateTimeValue("15/06/2024")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid date/time value")
	})

	t.Run("invalid_string", func(t *testing.T) {
		_, err := parseDateTimeValue("not-a-date")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid date/time value")
	})

	t.Run("empty_string", func(t *testing.T) {
		_, err := parseDateTimeValue("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid date/time value")
	})
}
