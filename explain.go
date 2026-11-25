// Copyright 2026 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gorm

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
)

const (
	maxExplainDepth = 16
	hexDigits       = "0123456789abcdef"
)

// isByteSliceType returns true when t is []byte or a named slice type with uint8 elements.
// Note: reflect.Type.ConvertibleTo([]byte) is true for string in Go, so we must check Kind explicitly.
func isByteSliceType(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8
}

// isNilReflectValue reports whether rv represents a nil value for kinds that can be nil.
func isNilReflectValue(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
		return rv.IsNil()
	default:
		return false
	}
}

// explainGoogleSQL substitutes positional '?' placeholders in sql with formatted
// Cloud Spanner GoogleSQL literals from vars, ignoring '?' inside string/bytes
// literals, backtick-quoted identifiers, and comments.
//
// TODO(googleapis/go-sql-spanner): Expose GoogleSQL-aware placeholder scanning
// or SQL explanation in go-sql-spanner/parser (StatementParser) so GORM does
// not have to maintain its own SQL literal and comment scanner.
//
// TODO(go-gorm-spanner): postgresql.Dialector embeds gorm.io/driver/postgres.Dialector
// and still uses logger.ExplainSQL; evaluate an analogous PostgreSQL-dialect
// implementation separately from GoogleSQL.
func explainGoogleSQL(sql string, vars ...interface{}) string {
	if strings.IndexByte(sql, '?') == -1 {
		return sql
	}

	var builder strings.Builder
	varIdx := 0
	lastCopied := 0

	for i := 0; i < len(sql); {
		switch sql[i] {
		case '\'', '"':
			i = skipGoogleSQLQuotedLiteral(sql, i)
		case '`':
			i = skipGoogleSQLSingleQuoted(sql, i+1, '`')
		case '-':
			if i+1 < len(sql) && sql[i+1] == '-' {
				i = skipGoogleSQLLineComment(sql, i+2)
			} else {
				i++
			}
		case '#':
			i = skipGoogleSQLLineComment(sql, i+1)
		case '/':
			if i+1 < len(sql) && sql[i+1] == '*' {
				i = skipGoogleSQLBlockComment(sql, i+2)
			} else {
				i++
			}
		case '?':
			if varIdx < len(vars) {
				if builder.Len() == 0 {
					builder.Grow(len(sql) + len(vars)*8)
				}
				builder.WriteString(sql[lastCopied:i])
				writeFormattedGoogleSQLValue(&builder, vars[varIdx], 0)
				varIdx++
				i++
				lastCopied = i
			} else {
				i++
			}
		default:
			i++
		}
	}

	if lastCopied == 0 {
		return sql
	}
	builder.WriteString(sql[lastCopied:])
	return builder.String()
}

func skipGoogleSQLQuotedLiteral(sql string, start int) int {
	quote := sql[start]
	if start+2 < len(sql) && sql[start+1] == quote && sql[start+2] == quote {
		return skipGoogleSQLTripleQuoted(sql, start+3, quote)
	}
	return skipGoogleSQLSingleQuoted(sql, start+1, quote)
}

// skipGoogleSQLSingleQuoted scans a single-line quoted literal or backtick identifier.
// GoogleSQL forbids unescaped newlines inside non-triple-quoted literals and identifiers,
// so scanning stops at '\n' to avoid pairing an unclosed quote across lines.
func skipGoogleSQLSingleQuoted(sql string, start int, quote byte) int {
	for i := start; i < len(sql); {
		if sql[i] == '\\' {
			if i+1 < len(sql) && sql[i+1] == '\n' {
				return i + 1
			}
			i = advancePastEscape(sql, i)
			continue
		}
		if sql[i] == quote {
			return i + 1
		}
		if sql[i] == '\n' {
			return i
		}
		i++
	}
	return len(sql)
}

func skipGoogleSQLTripleQuoted(sql string, start int, quote byte) int {
	for i := start; i < len(sql); {
		if sql[i] == '\\' {
			i = advancePastEscape(sql, i)
			continue
		}
		if sql[i] == quote && i+2 < len(sql) && sql[i+1] == quote && sql[i+2] == quote {
			return i + 3
		}
		i++
	}
	return len(sql)
}

func advancePastEscape(sql string, backslashPos int) int {
	if backslashPos+1 >= len(sql) {
		return len(sql)
	}
	if sql[backslashPos+1] >= 0x80 {
		_, size := utf8.DecodeRuneInString(sql[backslashPos+1:])
		return backslashPos + 1 + size
	}
	return backslashPos + 2
}

func skipGoogleSQLLineComment(sql string, start int) int {
	if idx := strings.IndexByte(sql[start:], '\n'); idx >= 0 {
		return start + idx + 1
	}
	return len(sql)
}

func skipGoogleSQLBlockComment(sql string, start int) int {
	if idx := strings.Index(sql[start:], "*/"); idx >= 0 {
		return start + idx + 2
	}
	return len(sql)
}

// writeFormattedGoogleSQLValue formats v as a GoogleSQL literal.
//
// Concrete handling is kept minimal to Go primitives plus Spanner types that
// require typed literals or special null/panic handling (time.Time, civil.Date,
// big.Rat, spanner.NullJSON, spanner.NullProtoMessage, spanner.NullProtoEnum);
// other wrappers rely on driver.Valuer, spanner.NullableValue, and fmt.Stringer.
//
// Intentionally left out of scope for now:
//   - Typed empty arrays (ARRAY<T>[]); slices emit ARRAY[...] so GoogleSQL infers
//     element types for non-empty arrays without maintaining a Go-to-Spanner type map.
//   - Typed UUID/INTERVAL literals (spanner.NullUUID formats via driver.Valuer /
//     Stringer as a string literal; spanner.Interval / NullInterval format as strings).
//   - CommitTimestamp (resolved via GORM's Valuer into PENDING_COMMIT_TIMESTAMP()
//     before Explain is called) and PostgreSQL-dialect types (PGNumeric, PGJsonB).
//
// TODO(googleapis/cloud.google.com/go/spanner): Add a SQL literal formatting
// helper or value-unwrapping interface in cloud.google.com/go/spanner so wrapper
// types can be formatted into typed GoogleSQL literals without type switches here.
func writeFormattedGoogleSQLValue(builder *strings.Builder, v interface{}, depth int) {
	if depth > maxExplainDepth || v == nil {
		builder.WriteString("NULL")
		return
	}

	switch val := v.(type) {
	case bool:
		builder.WriteString(strconv.FormatBool(val))
		return
	case string:
		writeGoogleSQLStringLiteral(builder, val)
		return
	case []byte:
		if val == nil {
			builder.WriteString("NULL")
		} else {
			writeGoogleSQLBytesLiteral(builder, val)
		}
		return
	case int:
		builder.WriteString(strconv.Itoa(val))
		return
	case int8:
		builder.WriteString(strconv.FormatInt(int64(val), 10))
		return
	case int16:
		builder.WriteString(strconv.FormatInt(int64(val), 10))
		return
	case int32:
		builder.WriteString(strconv.FormatInt(int64(val), 10))
		return
	case int64:
		builder.WriteString(strconv.FormatInt(val, 10))
		return
	case uint:
		builder.WriteString(strconv.FormatUint(uint64(val), 10))
		return
	case uint8:
		builder.WriteString(strconv.FormatUint(uint64(val), 10))
		return
	case uint16:
		builder.WriteString(strconv.FormatUint(uint64(val), 10))
		return
	case uint32:
		builder.WriteString(strconv.FormatUint(uint64(val), 10))
		return
	case uint64:
		builder.WriteString(strconv.FormatUint(val, 10))
		return
	case float32:
		writeGoogleSQLFloat(builder, float64(val), 32)
		return
	case float64:
		writeGoogleSQLFloat(builder, val, 64)
		return
	case time.Time:
		writeGoogleSQLTimestamp(builder, val)
		return
	case *time.Time:
		if val == nil {
			builder.WriteString("NULL")
		} else {
			writeGoogleSQLTimestamp(builder, *val)
		}
		return
	case civil.Date:
		writeGoogleSQLDate(builder, val)
		return
	case *civil.Date:
		if val == nil {
			builder.WriteString("NULL")
		} else {
			writeGoogleSQLDate(builder, *val)
		}
		return
	case big.Rat:
		writeGoogleSQLNumeric(builder, &val)
		return
	case *big.Rat:
		if val == nil {
			builder.WriteString("NULL")
		} else {
			writeGoogleSQLNumeric(builder, val)
		}
		return
	case spanner.NullJSON:
		if !val.Valid {
			builder.WriteString("NULL")
		} else {
			writeGoogleSQLJSON(builder, val.Value)
		}
		return
	case *spanner.NullJSON:
		if val == nil || !val.Valid {
			builder.WriteString("NULL")
		} else {
			writeGoogleSQLJSON(builder, val.Value)
		}
		return
	case spanner.NullProtoMessage:
		// NullProtoMessage represents NULL either with Valid=false or Valid=true and a typed nil message;
		// calling String() on a typed nil proto panics inside protoadapt.
		if !val.Valid || val.ProtoMessageVal == nil || isNilReflectValue(reflect.ValueOf(val.ProtoMessageVal)) {
			builder.WriteString("NULL")
			return
		}
	case *spanner.NullProtoMessage:
		if val == nil || !val.Valid || val.ProtoMessageVal == nil || isNilReflectValue(reflect.ValueOf(val.ProtoMessageVal)) {
			builder.WriteString("NULL")
			return
		}
	case spanner.NullProtoEnum:
		if !val.Valid || val.ProtoEnumVal == nil || isNilReflectValue(reflect.ValueOf(val.ProtoEnumVal)) {
			builder.WriteString("NULL")
			return
		}
	case *spanner.NullProtoEnum:
		if val == nil || !val.Valid || val.ProtoEnumVal == nil || isNilReflectValue(reflect.ValueOf(val.ProtoEnumVal)) {
			builder.WriteString("NULL")
			return
		}
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		builder.WriteString("NULL")
		return
	}

	if valuer, ok := v.(driver.Valuer); ok {
		val, err := safeValuerValue(valuer)
		if err != nil {
			builder.WriteString("NULL")
			return
		}
		writeFormattedGoogleSQLValue(builder, val, depth+1)
		return
	}

	if nullable, ok := v.(spanner.NullableValue); ok && nullable.IsNull() {
		builder.WriteString("NULL")
		return
	}

	if isNilReflectValue(rv) {
		builder.WriteString("NULL")
		return
	}

	if stringer, ok := v.(fmt.Stringer); ok {
		writeFormattedStringer(builder, reflect.Indirect(rv), stringer)
		return
	}

	if rv.Kind() == reflect.Ptr {
		writeFormattedGoogleSQLValue(builder, rv.Elem().Interface(), depth+1)
		return
	}

	if isByteSliceType(rv.Type()) {
		writeGoogleSQLBytesLiteral(builder, rv.Bytes())
		return
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		writeGoogleSQLArray(builder, rv, depth)
	case reflect.Bool:
		builder.WriteString(strconv.FormatBool(rv.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		builder.WriteString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		builder.WriteString(strconv.FormatUint(rv.Uint(), 10))
	case reflect.Float32:
		writeGoogleSQLFloat(builder, rv.Float(), 32)
	case reflect.Float64:
		writeGoogleSQLFloat(builder, rv.Float(), 64)
	case reflect.String:
		writeGoogleSQLStringLiteral(builder, rv.String())
	default:
		writeGoogleSQLStringLiteral(builder, fmt.Sprint(v))
	}
}

func safeValuerValue(valuer driver.Valuer) (val driver.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			val = nil
			err = fmt.Errorf("driver.Valuer panic: %v", r)
		}
	}()
	return valuer.Value()
}

func safeStringerString(stringer fmt.Stringer) (res string, ok bool) {
	defer func() {
		if recover() != nil {
			res = ""
			ok = false
		}
	}()
	return stringer.String(), true
}

func writeFormattedStringer(builder *strings.Builder, rv reflect.Value, stringer fmt.Stringer) {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		builder.WriteString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		builder.WriteString(strconv.FormatUint(rv.Uint(), 10))
	case reflect.Float32:
		writeGoogleSQLFloat(builder, rv.Float(), 32)
	case reflect.Float64:
		writeGoogleSQLFloat(builder, rv.Float(), 64)
	case reflect.Bool:
		builder.WriteString(strconv.FormatBool(rv.Bool()))
	default:
		text, ok := safeStringerString(stringer)
		if !ok {
			builder.WriteString("NULL")
			return
		}
		writeGoogleSQLStringLiteral(builder, text)
	}
}

func writeGoogleSQLArray(builder *strings.Builder, rv reflect.Value, depth int) {
	builder.WriteString("ARRAY[")
	for i := 0; i < rv.Len(); i++ {
		if i > 0 {
			builder.WriteString(", ")
		}
		writeFormattedGoogleSQLValue(builder, rv.Index(i).Interface(), depth+1)
	}
	builder.WriteByte(']')
}

func writeGoogleSQLFloat(builder *strings.Builder, v float64, bitSize int) {
	typeName := "FLOAT64"
	if bitSize == 32 {
		typeName = "FLOAT32"
	}
	switch {
	case math.IsNaN(v):
		builder.WriteString("CAST('NaN' AS ")
		builder.WriteString(typeName)
		builder.WriteByte(')')
	case math.IsInf(v, 1):
		builder.WriteString("CAST('Infinity' AS ")
		builder.WriteString(typeName)
		builder.WriteByte(')')
	case math.IsInf(v, -1):
		builder.WriteString("CAST('-Infinity' AS ")
		builder.WriteString(typeName)
		builder.WriteByte(')')
	default:
		formatted := strconv.FormatFloat(v, 'g', -1, bitSize)
		builder.WriteString(formatted)
		if !strings.ContainsAny(formatted, ".eE") {
			builder.WriteString(".0")
		}
	}
}

func writeGoogleSQLTimestamp(builder *strings.Builder, t time.Time) {
	builder.WriteString("TIMESTAMP ")
	writeGoogleSQLStringLiteral(builder, t.UTC().Format("2006-01-02 15:04:05.000000-07:00"))
}

func writeGoogleSQLDate(builder *strings.Builder, d civil.Date) {
	builder.WriteString("DATE ")
	writeGoogleSQLStringLiteral(builder, d.String())
}

func writeGoogleSQLNumeric(builder *strings.Builder, r *big.Rat) {
	builder.WriteString("NUMERIC ")
	writeGoogleSQLStringLiteral(builder, spanner.NumericString(r))
}

func writeGoogleSQLJSON(builder *strings.Builder, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		builder.WriteString("NULL")
		return
	}
	builder.WriteString("JSON ")
	writeGoogleSQLStringLiteral(builder, string(data))
}

func writeGoogleSQLStringLiteral(builder *strings.Builder, s string) {
	builder.WriteByte('\'')
	for i := 0; i < len(s); {
		b := s[i]
		if b < 0x80 {
			writeGoogleSQLEscapedByte(builder, b)
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			writeHexEscape(builder, b)
			i++
			continue
		}
		builder.WriteString(s[i : i+size])
		i += size
	}
	builder.WriteByte('\'')
}

func writeGoogleSQLBytesLiteral(builder *strings.Builder, data []byte) {
	builder.WriteString("b'")
	for _, b := range data {
		writeGoogleSQLEscapedByte(builder, b)
	}
	builder.WriteByte('\'')
}

func writeGoogleSQLEscapedByte(builder *strings.Builder, b byte) {
	switch b {
	case '\\':
		builder.WriteString(`\\`)
	case '\'':
		builder.WriteString(`\'`)
	case '\a':
		builder.WriteString(`\a`)
	case '\b':
		builder.WriteString(`\b`)
	case '\f':
		builder.WriteString(`\f`)
	case '\n':
		builder.WriteString(`\n`)
	case '\r':
		builder.WriteString(`\r`)
	case '\t':
		builder.WriteString(`\t`)
	case '\v':
		builder.WriteString(`\v`)
	default:
		if b < 0x20 || b >= 0x7f {
			writeHexEscape(builder, b)
		} else {
			builder.WriteByte(b)
		}
	}
}

func writeHexEscape(builder *strings.Builder, b byte) {
	builder.WriteString(`\x`)
	builder.WriteByte(hexDigits[b>>4])
	builder.WriteByte(hexDigits[b&0x0f])
}
