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
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/spansql"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type customString string
type customInt int64
type customUint uint64
type customFloat32 float32
type customFloat float64
type customBool bool
type customBytes []byte
type customUint8 uint8

type intStringer int64

func (i intStringer) String() string {
	return fmt.Sprintf("custom-%d", i)
}

type uintStringer uint64

func (u uintStringer) String() string {
	return fmt.Sprintf("u-%d", u)
}

type float32Stringer float32

func (f float32Stringer) String() string {
	return fmt.Sprintf("f32-%v", float32(f))
}

type float64Stringer float64

func (f float64Stringer) String() string {
	return fmt.Sprintf("f64-%v", float64(f))
}

type boolStringer bool

func (b boolStringer) String() string {
	return fmt.Sprintf("b-%v", bool(b))
}

type strStringer string

func (s strStringer) String() string {
	return "ignored-" + string(s)
}

type structStringer struct {
	text string
}

func (s structStringer) String() string {
	return s.text
}

type ptrStringer struct {
	text string
}

func (p *ptrStringer) String() string {
	return p.text
}

type cyclicValuer struct{}

func (c cyclicValuer) Value() (driver.Value, error) {
	return c, nil
}

type errorValuer struct{}

func (e errorValuer) Value() (driver.Value, error) {
	return nil, errors.New("valuer error")
}

type panickingValuer struct{}

func (p panickingValuer) Value() (driver.Value, error) {
	panic("boom valuer")
}

type panickingStringer struct{}

func (p panickingStringer) String() string {
	panic("boom stringer")
}

type customNullable struct {
	val   string
	valid bool
}

func (c customNullable) IsNull() bool {
	return !c.valid
}

func (c customNullable) String() string {
	return c.val
}

func TestDialector_Explain_StringEscaping(t *testing.T) {
	t.Parallel()
	dialector := Dialector{Config: &Config{}}

	tests := []struct {
		name string
		sql  string
		vars []interface{}
		want string
	}{
		{
			name: "plain string",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{"Alice"},
			want: "SELECT * FROM singers WHERE name = 'Alice'",
		},
		{
			name: "single quote escaped with backslash",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{"O'Reilly"},
			want: `SELECT * FROM singers WHERE name = 'O\'Reilly'`,
		},
		{
			name: "backslash escaped",
			sql:  "SELECT * FROM singers WHERE path = ?",
			vars: []interface{}{`a\b\`},
			want: `SELECT * FROM singers WHERE path = 'a\\b\\'`,
		},
		{
			name: "sqli backslash quote breakout blocked",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{`\'; DROP TABLE singers; --`},
			want: `SELECT * FROM singers WHERE name = '\\\'; DROP TABLE singers; --'`,
		},
		{
			name: "sqli single quote breakout blocked",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{"'; DROP TABLE singers; --"},
			want: `SELECT * FROM singers WHERE name = '\'; DROP TABLE singers; --'`,
		},
		{
			name: "null byte escaped as hex not octal",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{"a\x00b"},
			want: `SELECT * FROM singers WHERE name = 'a\x00b'`,
		},
		{
			name: "named control characters escaped",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{"\a\b\f\n\r\t\v"},
			want: `SELECT * FROM singers WHERE name = '\a\b\f\n\r\t\v'`,
		},
		{
			name: "other control characters escaped as hex",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{"\x01\x1b\x7f"},
			want: `SELECT * FROM singers WHERE name = '\x01\x1b\x7f'`,
		},
		{
			name: "utf8 preserved and invalid utf8 hex escaped",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{"héllo 世界 \xff"},
			want: `SELECT * FROM singers WHERE name = 'héllo 世界 \xff'`,
		},
		{
			name: "double quotes and backticks preserved inside single quotes",
			sql:  "SELECT * FROM singers WHERE name = ?",
			vars: []interface{}{`"quoted" and ` + "`bt`"},
			want: `SELECT * FROM singers WHERE name = '"quoted" and ` + "`bt`" + `'`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if g, w := dialector.Explain(tc.sql, tc.vars...), tc.want; g != w {
				t.Errorf("Explain mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestDialector_Explain_Bytes(t *testing.T) {
	t.Parallel()
	dialector := Dialector{Config: &Config{}}

	tests := []struct {
		name string
		val  interface{}
		want string
	}{
		{
			name: "printable bytes",
			val:  []byte("hello'world\\test"),
			want: `SELECT b'hello\'world\\test'`,
		},
		{
			name: "empty non-nil bytes",
			val:  []byte{},
			want: "SELECT b''",
		},
		{
			name: "nil bytes",
			val:  []byte(nil),
			want: "SELECT NULL",
		},
		{
			name: "binary bytes with null and non-ascii",
			val:  []byte{0x00, 0x01, 0x0a, 0x1f, 0x20, 0x7e, 0x7f, 0x80, 0xff},
			want: `SELECT b'\x00\x01\n\x1f ~\x7f\x80\xff'`,
		},
		{
			name: "custom byte slice type",
			val:  customBytes{0x00, 'a', '\''},
			want: `SELECT b'\x00a\''`,
		},
		{
			name: "nil custom byte slice type",
			val:  customBytes(nil),
			want: "SELECT NULL",
		},
		{
			name: "slice of custom uint8 element type",
			val:  []customUint8{0x00, 'b', '\''},
			want: `SELECT b'\x00b\''`,
		},
		{
			name: "sql.RawBytes",
			val:  sql.RawBytes("raw'bytes"),
			want: `SELECT b'raw\'bytes'`,
		},
		{
			name: "fixed byte array formatted as ARRAY",
			val:  [4]byte{1, 2, 3, 4},
			want: "SELECT ARRAY[1, 2, 3, 4]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if g, w := dialector.Explain("SELECT ?", tc.val), tc.want; g != w {
				t.Errorf("Explain bytes mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestDialector_Explain_PlaceholdersIgnoredInLiteralsAndComments(t *testing.T) {
	t.Parallel()
	dialector := Dialector{Config: &Config{}}

	tests := []struct {
		name string
		sql  string
		vars []interface{}
		want string
	}{
		{
			name: "empty sql",
			sql:  "",
			vars: []interface{}{1},
			want: "",
		},
		{
			name: "nil vars leaves placeholder untouched",
			sql:  "SELECT ?",
			vars: nil,
			want: "SELECT ?",
		},
		{
			name: "question mark inside single quoted string",
			sql:  "SELECT * FROM t WHERE a = 'what?' AND b = ?",
			vars: []interface{}{42},
			want: "SELECT * FROM t WHERE a = 'what?' AND b = 42",
		},
		{
			name: "empty literals and backtick identifier",
			sql:  "SELECT '', \"\", '''''', `` FROM t WHERE id = ?",
			vars: []interface{}{1},
			want: "SELECT '', \"\", '''''', `` FROM t WHERE id = 1",
		},
		{
			name: "escaped quote before question mark inside string",
			sql:  `SELECT * FROM t WHERE a = 'it\'s?' AND b = ?`,
			vars: []interface{}{"ok"},
			want: `SELECT * FROM t WHERE a = 'it\'s?' AND b = 'ok'`,
		},
		{
			name: "escaped backslash at end of string before placeholder",
			sql:  `SELECT * FROM t WHERE a = 'trail\\' AND b = ?`,
			vars: []interface{}{"ok"},
			want: `SELECT * FROM t WHERE a = 'trail\\' AND b = 'ok'`,
		},
		{
			name: "question mark inside double quoted string",
			sql:  `SELECT * FROM t WHERE a = "why?" AND b = ?`,
			vars: []interface{}{true},
			want: `SELECT * FROM t WHERE a = "why?" AND b = true`,
		},
		{
			name: "question mark inside triple single quoted string",
			sql:  "SELECT * FROM t WHERE a = '''line1?\n''still?inside''' AND b = ?",
			vars: []interface{}{7},
			want: "SELECT * FROM t WHERE a = '''line1?\n''still?inside''' AND b = 7",
		},
		{
			name: "question mark inside triple double quoted string",
			sql:  `SELECT * FROM t WHERE a = """line1?""still?inside""" AND b = ?`,
			vars: []interface{}{8},
			want: `SELECT * FROM t WHERE a = """line1?""still?inside""" AND b = 8`,
		},
		{
			name: "question mark inside lowercase and uppercase raw and bytes literals",
			sql:  `SELECT r'raw?', R'RAW?', b'bytes?', B"BYTES?", rb"rb?", BR'br?', RB'''rbt?''' FROM t WHERE id = ?`,
			vars: []interface{}{9},
			want: `SELECT r'raw?', R'RAW?', b'bytes?', B"BYTES?", rb"rb?", BR'br?', RB'''rbt?''' FROM t WHERE id = 9`,
		},
		{
			name: "raw string ending in escaped backslash before placeholder",
			sql:  `SELECT r'a\\' FROM t WHERE id = ?`,
			vars: []interface{}{99},
			want: `SELECT r'a\\' FROM t WHERE id = 99`,
		},
		{
			name: "question mark inside backtick identifier",
			sql:  "SELECT `col?1`, `col\\`?2`, `a\\\\` FROM `tbl?` WHERE id = ?",
			vars: []interface{}{10},
			want: "SELECT `col?1`, `col\\`?2`, `a\\\\` FROM `tbl?` WHERE id = 10",
		},
		{
			name: "question mark inside dash line comment",
			sql:  "SELECT * FROM t -- is this ? ignored\nWHERE id = ?",
			vars: []interface{}{11},
			want: "SELECT * FROM t -- is this ? ignored\nWHERE id = 11",
		},
		{
			name: "question mark inside hash line comment",
			sql:  "SELECT * FROM t # is this ? ignored\nWHERE id = ?",
			vars: []interface{}{12},
			want: "SELECT * FROM t # is this ? ignored\nWHERE id = 12",
		},
		{
			name: "question mark inside block comment and comment chars inside string",
			sql:  "SELECT /* ' ? */ '/* ?' FROM t WHERE id = ?",
			vars: []interface{}{13},
			want: "SELECT /* ' ? */ '/* ?' FROM t WHERE id = 13",
		},
		{
			name: "multi-statement sql",
			sql:  "SELECT 'a?'; SELECT ?",
			vars: []interface{}{21},
			want: "SELECT 'a?'; SELECT 21",
		},
		{
			name: "parameter value containing question mark not re-scanned",
			sql:  "SELECT ?, ?",
			vars: []interface{}{"who?", "second"},
			want: "SELECT 'who?', 'second'",
		},
		{
			name: "fewer vars than placeholders leaves trailing placeholder",
			sql:  "SELECT ?, ?",
			vars: []interface{}{1},
			want: "SELECT 1, ?",
		},
		{
			name: "more vars than placeholders ignores extra vars",
			sql:  "SELECT ?",
			vars: []interface{}{1, 2},
			want: "SELECT 1",
		},
		{
			name: "unclosed single line literal stops at newline",
			sql:  "SELECT 'unclosed\nWHERE a = 'x' AND b = ?",
			vars: []interface{}{5},
			want: "SELECT 'unclosed\nWHERE a = 'x' AND b = 5",
		},
		{
			name: "unclosed literal does not panic and preserves sql",
			sql:  "SELECT 'unclosed ?",
			vars: []interface{}{1},
			want: "SELECT 'unclosed ?",
		},
		{
			name: "unclosed block comment does not panic and preserves sql",
			sql:  "SELECT /* unclosed ?",
			vars: []interface{}{1},
			want: "SELECT /* unclosed ?",
		},
		{
			name: "only literal question marks returns sql unchanged",
			sql:  "SELECT '?', -- ?\n/* ? */",
			vars: []interface{}{1},
			want: "SELECT '?', -- ?\n/* ? */",
		},
		{
			name: "single dash and slash operators before placeholder",
			sql:  "SELECT a - b / c FROM t WHERE id = ?",
			vars: []interface{}{14},
			want: "SELECT a - b / c FROM t WHERE id = 14",
		},
		{
			name: "trailing dash and slash at EOF after placeholder",
			sql:  "SELECT ? -",
			vars: []interface{}{1},
			want: "SELECT 1 -",
		},
		{
			name: "trailing slash at EOF after placeholder",
			sql:  "SELECT ? /",
			vars: []interface{}{2},
			want: "SELECT 2 /",
		},
		{
			name: "escaped triple quote inside triple quoted string",
			sql:  `SELECT * FROM t WHERE a = '''foo\'''bar''' AND b = ?`,
			vars: []interface{}{15},
			want: `SELECT * FROM t WHERE a = '''foo\'''bar''' AND b = 15`,
		},
		{
			name: "unclosed triple quoted string does not panic",
			sql:  "SELECT '''unclosed ?",
			vars: []interface{}{1},
			want: "SELECT '''unclosed ?",
		},
		{
			name: "trailing backslash at EOF inside literal",
			sql:  `SELECT ?, 'unclosed\`,
			vars: []interface{}{1},
			want: `SELECT 1, 'unclosed\`,
		},
		{
			name: "backslash before multibyte utf8 rune inside literal",
			sql:  `SELECT 'a\éb?' WHERE id = ?`,
			vars: []interface{}{16},
			want: `SELECT 'a\éb?' WHERE id = 16`,
		},
		{
			name: "line comment at EOF without newline",
			sql:  "SELECT * FROM t -- ?",
			vars: []interface{}{1},
			want: "SELECT * FROM t -- ?",
		},
		{
			name: "no placeholders returns sql unchanged",
			sql:  "SELECT * FROM t",
			vars: []interface{}{1},
			want: "SELECT * FROM t",
		},
		{
			name: "dangling backslash before newline in single-line literal stops at newline",
			sql:  "SELECT 'unclosed\\\nWHERE id = ?",
			vars: []interface{}{17},
			want: "SELECT 'unclosed\\\nWHERE id = 17",
		},
		{
			name: "double quoted escaped quotes and trailing backslash",
			sql:  `SELECT "a\"?", "trail\\" FROM t WHERE id = ?`,
			vars: []interface{}{18},
			want: `SELECT "a\"?", "trail\\" FROM t WHERE id = 18`,
		},
		{
			name: "triple double quoted multiline with escapes",
			sql:  "SELECT \"\"\"a\\\"\n\"\"?\"\"\" FROM t WHERE id = ?",
			vars: []interface{}{19},
			want: "SELECT \"\"\"a\\\"\n\"\"?\"\"\" FROM t WHERE id = 19",
		},
		{
			name: "hash comment at EOF without newline",
			sql:  "SELECT * FROM t # ?",
			vars: []interface{}{1},
			want: "SELECT * FROM t # ?",
		},
		{
			name: "block comment /*/*/ closes at */",
			sql:  "SELECT /*/*/ ?",
			vars: []interface{}{20},
			want: "SELECT /*/*/ 20",
		},
		{
			name: "unclosed raw string with trailing backslash",
			sql:  `SELECT r'a\' ?`,
			vars: []interface{}{1},
			want: `SELECT r'a\' ?`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if g, w := dialector.Explain(tc.sql, tc.vars...), tc.want; g != w {
				t.Errorf("Explain mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestDialector_Explain_Arrays(t *testing.T) {
	t.Parallel()
	dialector := Dialector{Config: &Config{}}

	ts := time.Date(2025, 2, 10, 12, 55, 7, 974000000, time.UTC)
	d := civil.Date{Year: 2025, Month: 2, Day: 10}
	s1 := "a'b"

	tests := []struct {
		name string
		val  interface{}
		want string
	}{
		{
			name: "StringArray with escaping",
			val:  StringArray{"one", "two'three\\four", "five\x00six"},
			want: `SELECT ARRAY['one', 'two\'three\\four', 'five\x00six']`,
		},
		{
			name: "empty StringArray",
			val:  StringArray{},
			want: "SELECT ARRAY[]",
		},
		{
			name: "nil StringArray",
			val:  StringArray(nil),
			want: "SELECT NULL",
		},
		{
			name: "NullStringArray",
			val:  NullStringArray{{StringVal: "a'b", Valid: true}, {}},
			want: `SELECT ARRAY['a\'b', NULL]`,
		},
		{
			name: "pointer string slice",
			val:  []*string{&s1, nil},
			want: `SELECT ARRAY['a\'b', NULL]`,
		},
		{
			name: "BoolArray",
			val:  BoolArray{true, false},
			want: "SELECT ARRAY[true, false]",
		},
		{
			name: "NullBoolArray",
			val:  NullBoolArray{{Bool: true, Valid: true}, {}},
			want: "SELECT ARRAY[true, NULL]",
		},
		{
			name: "Int64Array",
			val:  Int64Array{-42, 99},
			want: "SELECT ARRAY[-42, 99]",
		},
		{
			name: "NullInt64Array",
			val:  NullInt64Array{{Int64: 10, Valid: true}, {}},
			want: "SELECT ARRAY[10, NULL]",
		},
		{
			name: "Float32Array",
			val:  Float32Array{3.14, float32(math.NaN())},
			want: "SELECT ARRAY[3.14, CAST('NaN' AS FLOAT32)]",
		},
		{
			name: "NullFloat32Array",
			val:  NullFloat32Array{{Float32: 1.5, Valid: true}, {}},
			want: "SELECT ARRAY[1.5, NULL]",
		},
		{
			name: "Float64Array",
			val:  Float64Array{1.0, 2.5},
			want: "SELECT ARRAY[1.0, 2.5]",
		},
		{
			name: "NullFloat64Array",
			val:  NullFloat64Array{{Float64: 2.0, Valid: true}, {}},
			want: "SELECT ARRAY[2.0, NULL]",
		},
		{
			name: "BytesArray",
			val:  BytesArray{[]byte("a'b"), nil, []byte{0x00, 0xff}},
			want: `SELECT ARRAY[b'a\'b', NULL, b'\x00\xff']`,
		},
		{
			name: "NullBytesArray",
			val:  NullBytesArray{[]byte("x"), nil},
			want: "SELECT ARRAY[b'x', NULL]",
		},
		{
			name: "DateArray",
			val:  DateArray{d},
			want: "SELECT ARRAY[DATE '2025-02-10']",
		},
		{
			name: "NullDateArray",
			val:  NullDateArray{{Date: d, Valid: true}, {}},
			want: "SELECT ARRAY[DATE '2025-02-10', NULL]",
		},
		{
			name: "TimeArray",
			val:  TimeArray{ts},
			want: "SELECT ARRAY[TIMESTAMP '2025-02-10 12:55:07.974000+00:00']",
		},
		{
			name: "NullTimeArray",
			val:  NullTimeArray{{Time: ts, Valid: true}, {}},
			want: "SELECT ARRAY[TIMESTAMP '2025-02-10 12:55:07.974000+00:00', NULL]",
		},
		{
			name: "NullJSONArray",
			val:  NullJSONArray{{Value: map[string]string{"k": "v'1"}, Valid: true}, {}},
			want: `SELECT ARRAY[JSON '{"k":"v\'1"}', NULL]`,
		},
		{
			name: "fixed Go array",
			val:  [2]int64{10, 20},
			want: "SELECT ARRAY[10, 20]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if g, w := dialector.Explain("SELECT ?", tc.val), tc.want; g != w {
				t.Errorf("Explain array mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestDialector_Explain_PrimitivesAndNullables(t *testing.T) {
	t.Parallel()
	dialector := Dialector{Config: &Config{}}

	loc := time.FixedZone("CET", 3600)
	ts := time.Date(2025, 2, 17, 11, 14, 0, 123456000, loc)
	d := civil.Date{Year: 2025, Month: 2, Day: 17}
	rat := big.NewRat(3, 2)
	iv, err := spanner.ParseInterval("P1Y2M3D")
	if err != nil {
		t.Fatalf("ParseInterval failed: %v", err)
	}

	tests := []struct {
		name string
		val  interface{}
		want string
	}{
		{name: "nil", val: nil, want: "NULL"},
		{name: "bool true", val: true, want: "true"},
		{name: "bool false", val: false, want: "false"},
		{name: "int", val: int(-123), want: "-123"},
		{name: "int8", val: int8(-8), want: "-8"},
		{name: "int16", val: int16(-16), want: "-16"},
		{name: "int32", val: int32(-32), want: "-32"},
		{name: "int64 min", val: int64(math.MinInt64), want: "-9223372036854775808"},
		{name: "int64 max", val: int64(math.MaxInt64), want: "9223372036854775807"},
		{name: "uint", val: uint(42), want: "42"},
		{name: "uint8", val: uint8(8), want: "8"},
		{name: "uint16", val: uint16(16), want: "16"},
		{name: "uint32", val: uint32(32), want: "32"},
		{name: "uint64 max", val: uint64(math.MaxUint64), want: "18446744073709551615"},
		{name: "float32 finite", val: float32(1.5), want: "1.5"},
		{name: "float32 integer-like", val: float32(2), want: "2.0"},
		{name: "float32 +Inf", val: float32(math.Inf(1)), want: "CAST('Infinity' AS FLOAT32)"},
		{name: "float32 -Inf", val: float32(math.Inf(-1)), want: "CAST('-Infinity' AS FLOAT32)"},
		{name: "float32 NaN", val: float32(math.NaN()), want: "CAST('NaN' AS FLOAT32)"},
		{name: "float64 integer-like", val: float64(2), want: "2.0"},
		{name: "float64 negative zero", val: math.Copysign(0, -1), want: "-0.0"},
		{name: "float64 scientific notation", val: float64(1e25), want: "1e+25"},
		{name: "float64 NaN", val: math.NaN(), want: "CAST('NaN' AS FLOAT64)"},
		{name: "float64 +Inf", val: math.Inf(1), want: "CAST('Infinity' AS FLOAT64)"},
		{name: "float64 -Inf", val: math.Inf(-1), want: "CAST('-Infinity' AS FLOAT64)"},
		{name: "time UTC microsecond", val: ts, want: "TIMESTAMP '2025-02-17 10:14:00.123456+00:00'"},
		{name: "time negative offset and sub-microsecond truncation", val: time.Date(2025, 2, 17, 10, 14, 0, 123456789, time.FixedZone("EST", -5*3600)), want: "TIMESTAMP '2025-02-17 15:14:00.123456+00:00'"},
		{name: "non-nil pointer to time", val: &ts, want: "TIMESTAMP '2025-02-17 10:14:00.123456+00:00'"},
		{name: "zero time valid in spanner", val: time.Time{}, want: "TIMESTAMP '0001-01-01 00:00:00.000000+00:00'"},
		{name: "civil.Date", val: d, want: "DATE '2025-02-17'"},
		{name: "pointer civil.Date", val: &d, want: "DATE '2025-02-17'"},
		{name: "big.Rat", val: *rat, want: "NUMERIC '1.500000000'"},
		{name: "negative big.Rat", val: big.NewRat(-5, 4), want: "NUMERIC '-1.250000000'"},
		{name: "pointer big.Rat", val: rat, want: "NUMERIC '1.500000000'"},
		{name: "spanner.NullString valid", val: spanner.NullString{StringVal: "x'y", Valid: true}, want: `'x\'y'`},
		{name: "spanner.NullString null", val: spanner.NullString{}, want: "NULL"},
		{name: "spanner.NullInt64 valid", val: spanner.NullInt64{Int64: 123, Valid: true}, want: "123"},
		{name: "spanner.NullInt64 null", val: spanner.NullInt64{}, want: "NULL"},
		{name: "spanner.NullBool valid", val: spanner.NullBool{Bool: true, Valid: true}, want: "true"},
		{name: "spanner.NullBool null", val: spanner.NullBool{}, want: "NULL"},
		{name: "spanner.NullFloat32 valid", val: spanner.NullFloat32{Float32: 2.5, Valid: true}, want: "2.5"},
		{name: "spanner.NullFloat32 null", val: spanner.NullFloat32{}, want: "NULL"},
		{name: "spanner.NullFloat64 valid", val: spanner.NullFloat64{Float64: 4, Valid: true}, want: "4.0"},
		{name: "spanner.NullFloat64 null", val: spanner.NullFloat64{}, want: "NULL"},
		{name: "spanner.NullTime valid", val: spanner.NullTime{Time: ts, Valid: true}, want: "TIMESTAMP '2025-02-17 10:14:00.123456+00:00'"},
		{name: "spanner.NullTime null", val: spanner.NullTime{}, want: "NULL"},
		{name: "spanner.NullDate valid", val: spanner.NullDate{Date: d, Valid: true}, want: "DATE '2025-02-17'"},
		{name: "spanner.NullDate null", val: spanner.NullDate{}, want: "NULL"},
		{name: "spanner.NullNumeric valid", val: spanner.NullNumeric{Numeric: *rat, Valid: true}, want: "NUMERIC '1.500000000'"},
		{name: "spanner.NullNumeric valid zero rat", val: spanner.NullNumeric{Valid: true}, want: "NUMERIC '0.000000000'"},
		{name: "spanner.NullNumeric null", val: spanner.NullNumeric{}, want: "NULL"},
		{name: "spanner.NullJSON valid", val: spanner.NullJSON{Value: map[string]string{"a": "b'c"}, Valid: true}, want: `JSON '{"a":"b\'c"}'`},
		{name: "spanner.NullJSON unmarshalable", val: spanner.NullJSON{Value: math.NaN(), Valid: true}, want: "NULL"},
		{name: "spanner.NullJSON null", val: spanner.NullJSON{}, want: "NULL"},
		{name: "pointer spanner.NullJSON valid", val: &spanner.NullJSON{Value: "hi", Valid: true}, want: `JSON '"hi"'`},
		{name: "pointer spanner.NullJSON null", val: &spanner.NullJSON{}, want: "NULL"},
		{name: "spanner.NullUUID valid", val: spanner.NullUUID{Valid: true}, want: "'00000000-0000-0000-0000-000000000000'"},
		{name: "spanner.NullUUID null", val: spanner.NullUUID{}, want: "NULL"},
		{name: "spanner.NullInterval valid", val: spanner.NullInterval{Interval: iv, Valid: true}, want: "'P1Y2M3D'"},
		{name: "spanner.NullInterval null", val: spanner.NullInterval{}, want: "NULL"},
		{name: "spanner.NullProtoMessage null", val: spanner.NullProtoMessage{}, want: "NULL"},
		{name: "spanner.NullProtoMessage untyped nil valid true", val: spanner.NullProtoMessage{ProtoMessageVal: nil, Valid: true}, want: "NULL"},
		{name: "spanner.NullProtoMessage typed nil valid true", val: spanner.NullProtoMessage{ProtoMessageVal: (*emptypb.Empty)(nil), Valid: true}, want: "NULL"},
		{name: "pointer spanner.NullProtoMessage typed nil valid true", val: &spanner.NullProtoMessage{ProtoMessageVal: (*emptypb.Empty)(nil), Valid: true}, want: "NULL"},
		{name: "spanner.NullProtoMessage non-nil", val: spanner.NullProtoMessage{ProtoMessageVal: &emptypb.Empty{}, Valid: true}, want: "''"},
		{name: "pointer spanner.NullProtoMessage non-nil", val: &spanner.NullProtoMessage{ProtoMessageVal: &emptypb.Empty{}, Valid: true}, want: "''"},
		{name: "spanner.NullProtoEnum null", val: spanner.NullProtoEnum{}, want: "NULL"},
		{name: "pointer spanner.NullProtoEnum null", val: &spanner.NullProtoEnum{}, want: "NULL"},
		{name: "spanner.NullProtoEnum nil enum valid true", val: spanner.NullProtoEnum{ProtoEnumVal: nil, Valid: true}, want: "NULL"},
		{name: "pointer spanner.NullProtoEnum nil enum valid true", val: &spanner.NullProtoEnum{ProtoEnumVal: nil, Valid: true}, want: "NULL"},
		{name: "spanner.NullProtoEnum non-nil", val: spanner.NullProtoEnum{ProtoEnumVal: spannerpb.TypeCode_STRING, Valid: true}, want: "'STRING'"},
		{name: "pointer spanner.NullProtoEnum non-nil", val: &spanner.NullProtoEnum{ProtoEnumVal: spannerpb.TypeCode_STRING, Valid: true}, want: "'STRING'"},
		{name: "custom NullableValue null", val: customNullable{}, want: "NULL"},
		{name: "custom NullableValue valid", val: customNullable{val: "a'b", valid: true}, want: `'a\'b'`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if g, w := dialector.Explain("SELECT ?", tc.val), "SELECT "+tc.want; g != w {
				t.Errorf("Explain type mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestDialector_Explain_ValuersAndPointers(t *testing.T) {
	t.Parallel()
	dialector := Dialector{Config: &Config{}}

	strVal := "ptr'str"
	strPtr := &strVal
	intStr := intStringer(12)
	var nilStrPtr *string
	var nilTimePtr *time.Time
	var nilDatePtr *civil.Date
	var nilRatPtr *big.Rat
	var nilNullStrPtr *spanner.NullString
	var nilNullJSONPtr *spanner.NullJSON
	var nilNullProtoPtr *spanner.NullProtoMessage
	var nilNullEnumPtr *spanner.NullProtoEnum
	var nilValuer *sql.NullString
	var nilCustomBytesPtr *customBytes
	var nilPtrStringer *ptrStringer

	cyclicSlice := make([]interface{}, 1)
	cyclicSlice[0] = cyclicSlice

	tests := []struct {
		name string
		val  interface{}
		want string
	}{
		{name: "sql.NullString valid", val: sql.NullString{String: "a'b", Valid: true}, want: `'a\'b'`},
		{name: "sql.NullString null", val: sql.NullString{}, want: "NULL"},
		{name: "sql.NullInt64 valid", val: sql.NullInt64{Int64: 7, Valid: true}, want: "7"},
		{name: "nil pointer to string", val: nilStrPtr, want: "NULL"},
		{name: "non-nil pointer to string", val: &strVal, want: `'ptr\'str'`},
		{name: "double pointer to string", val: &strPtr, want: `'ptr\'str'`},
		{name: "nil pointer to time", val: nilTimePtr, want: "NULL"},
		{name: "nil pointer to date", val: nilDatePtr, want: "NULL"},
		{name: "nil pointer to rat", val: nilRatPtr, want: "NULL"},
		{name: "nil pointer to spanner.NullString", val: nilNullStrPtr, want: "NULL"},
		{name: "nil pointer to spanner.NullJSON", val: nilNullJSONPtr, want: "NULL"},
		{name: "nil pointer to spanner.NullProtoMessage", val: nilNullProtoPtr, want: "NULL"},
		{name: "nil pointer to spanner.NullProtoEnum", val: nilNullEnumPtr, want: "NULL"},
		{name: "nil pointer to driver.Valuer", val: nilValuer, want: "NULL"},
		{name: "nil pointer to customBytes", val: nilCustomBytesPtr, want: "NULL"},
		{name: "nil pointer to ptrStringer", val: nilPtrStringer, want: "NULL"},
		{name: "pointer to nil customBytes slice", val: new(customBytes), want: "NULL"},
		{name: "nil net.IP slice Stringer renders NULL", val: net.IP(nil), want: "NULL"},
		{name: "non-nil net.IP Stringer", val: net.IPv4(127, 0, 0, 1), want: "'127.0.0.1'"},
		{name: "nil interface slice", val: []interface{}(nil), want: "NULL"},
		{name: "empty interface slice", val: []interface{}{}, want: "ARRAY[]"},
		{name: "empty fixed array", val: [0]int64{}, want: "ARRAY[]"},
		{name: "slice containing error and panicking valuers", val: []interface{}{errorValuer{}, panickingValuer{}}, want: "ARRAY[NULL, NULL]"},
		{name: "error driver.Valuer", val: errorValuer{}, want: "NULL"},
		{name: "panicking driver.Valuer", val: panickingValuer{}, want: "NULL"},
		{name: "panicking fmt.Stringer", val: panickingStringer{}, want: "NULL"},
		{name: "cyclic driver.Valuer terminates safely", val: cyclicValuer{}, want: "NULL"},
		{name: "cyclic slice terminates safely", val: cyclicSlice, want: "ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[ARRAY[NULL]]]]]]]]]]]]]]]]]"},
		{name: "custom string type", val: customString("a'b"), want: `'a\'b'`},
		{name: "custom int type", val: customInt(55), want: "55"},
		{name: "custom uint type", val: customUint(66), want: "66"},
		{name: "custom float32 type", val: customFloat32(1.5), want: "1.5"},
		{name: "custom float type", val: customFloat(1.25), want: "1.25"},
		{name: "custom bool type", val: customBool(true), want: "true"},
		{name: "int stringer uses numeric value", val: intStringer(12), want: "12"},
		{name: "pointer to int stringer uses numeric value", val: &intStr, want: "12"},
		{name: "uint stringer uses numeric value", val: uintStringer(34), want: "34"},
		{name: "float32 stringer uses numeric value", val: float32Stringer(1.5), want: "1.5"},
		{name: "float64 stringer uses numeric value", val: float64Stringer(2.5), want: "2.5"},
		{name: "bool stringer uses bool value", val: boolStringer(true), want: "true"},
		{name: "string stringer calls String safely escaped", val: strStringer("a'b"), want: `'ignored-a\'b'`},
		{name: "struct stringer escaped as string", val: structStringer{text: "x'y\\z"}, want: `'x\'y\\z'`},
		{name: "pointer receiver stringer escaped as string", val: &ptrStringer{text: "p'q"}, want: `'p\'q'`},
		{name: "plain struct fallback escaped as string", val: struct{ Name string }{Name: "a'b"}, want: `'{a\'b}'`},
		{name: "map fallback escaped as string", val: map[string]string{"k'1": "v'2"}, want: `'map[k\'1:v\'2]'`},
		{name: "nil map renders NULL", val: map[string]string(nil), want: "NULL"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if g, w := dialector.Explain("SELECT ?", tc.val), "SELECT "+tc.want; g != w {
				t.Errorf("Explain valuer/pointer mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestDialector_Explain_SpansqlRoundTrip(t *testing.T) {
	t.Parallel()
	dialector := Dialector{Config: &Config{}}

	adversarialStrings := []string{
		"",
		"simple",
		"O'Reilly",
		`'`,
		`\`,
		`\'`,
		`\\`,
		`\'; DROP TABLE singers; --`,
		`'; DELETE FROM singers WHERE '1'='1`,
		"line1\nline2\r\nline3\ttab",
		"null\x00byte",
		"\a\b\f\n\r\t\v\x01\x1b\x7f",
		"unicode: café 日本語",
	}

	for _, s := range adversarialStrings {
		t.Run("string_"+fmt.Sprintf("%q", s), func(t *testing.T) {
			explained := dialector.Explain("SELECT * FROM `singers` WHERE `name` = ? AND `tag` = 'keep?'", s)
			parsed, err := spansql.ParseQuery(explained)
			if err != nil {
				t.Fatalf("spansql.ParseQuery failed for %s: %v", explained, err)
			}
			andExpr, ok := parsed.Select.Where.(spansql.LogicalOp)
			if !ok || andExpr.Op != spansql.And {
				t.Fatalf("unexpected WHERE AST type %T for %s", parsed.Select.Where, explained)
			}
			cmpOp, ok := andExpr.LHS.(spansql.ComparisonOp)
			if !ok {
				t.Fatalf("unexpected LHS AST type %T for %s", andExpr.LHS, explained)
			}
			strLit, ok := cmpOp.RHS.(spansql.StringLiteral)
			if !ok {
				t.Fatalf("unexpected RHS AST type %T for %s", cmpOp.RHS, explained)
			}
			if g, w := string(strLit), s; g != w {
				t.Errorf("round-trip string literal mismatch\nGot:  %q\nWant: %q", g, w)
			}
		})
	}

	adversarialBytes := [][]byte{
		{},
		[]byte("abc'def\\ghi"),
		{0x00, 0x01, 0x0a, 0x0d, 0x1b, 0x27, 0x5c, 0x7f, 0x80, 0xff},
	}

	for i, b := range adversarialBytes {
		t.Run(fmt.Sprintf("bytes_%d", i), func(t *testing.T) {
			explained := dialector.Explain("SELECT * FROM `blobs` WHERE `data` = ?", b)
			parsed, err := spansql.ParseQuery(explained)
			if err != nil {
				t.Fatalf("spansql.ParseQuery failed for %s: %v", explained, err)
			}
			cmpOp, ok := parsed.Select.Where.(spansql.ComparisonOp)
			if !ok {
				t.Fatalf("unexpected WHERE AST type %T for %s", parsed.Select.Where, explained)
			}
			bytesLit, ok := cmpOp.RHS.(spansql.BytesLiteral)
			if !ok {
				t.Fatalf("unexpected RHS AST type %T for %s", cmpOp.RHS, explained)
			}
			if g, w := string(bytesLit), string(b); g != w {
				t.Errorf("round-trip bytes literal mismatch\nGot:  %x\nWant: %x", g, w)
			}
		})
	}

	typedValues := []struct {
		name string
		val  interface{}
	}{
		{name: "timestamp", val: time.Date(2025, 2, 17, 10, 14, 0, 123456000, time.UTC)},
		{name: "date", val: civil.Date{Year: 2025, Month: 2, Day: 17}},
		{name: "float64", val: 3.14159},
		{name: "json", val: spanner.NullJSON{Value: map[string]string{"k": "v'1"}, Valid: true}},
		{name: "string_array", val: StringArray{"a'b", "c\\d"}},
		{name: "date_array", val: DateArray{{Year: 2025, Month: 2, Day: 17}}},
	}

	for _, tc := range typedValues {
		t.Run("typed_"+tc.name, func(t *testing.T) {
			explained := dialector.Explain("SELECT * FROM `t` WHERE `c` = ?", tc.val)
			if _, err := spansql.ParseQuery(explained); err != nil {
				t.Fatalf("spansql.ParseQuery failed for %s: %v", explained, err)
			}
		})
	}
}

func TestDialector_Explain_ToSQLAndDefaultValue(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	// Test gorm.DB.ToSQL with adversarial string values.
	sqlStr := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Table("singers").Where("first_name = ? AND last_name = ?", `O'Reilly\`, "a\x00b").Find(&singer{})
	})
	wantSQL := "SELECT * FROM `singers` WHERE (first_name = 'O\\'Reilly\\\\' AND last_name = 'a\\x00b') AND `singers`.`deleted_at` IS NULL"
	if g, w := sqlStr, wantSQL; g != w {
		t.Errorf("ToSQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
	if _, err := spansql.ParseQuery(sqlStr); err != nil {
		t.Fatalf("spansql.ParseQuery failed on ToSQL output %s: %v", sqlStr, err)
	}

	// Test AutoMigrate DDL generation when columns have default values containing quotes or injection payloads.
	type defaultModel struct {
		ID      int64
		Name    string `gorm:"default:o'reilly"`
		Payload string `gorm:"default:\\' OR 1=1 --"`
	}
	anyProto, err := anypb.New(&emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	server.TestDatabaseAdmin.SetResps([]proto.Message{
		&longrunningpb.Operation{
			Name:   "test-operation",
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyProto},
		},
	})
	if err := db.Migrator().AutoMigrate(&defaultModel{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	reqs := server.TestDatabaseAdmin.Reqs()
	if g, w := len(reqs), 1; g != w {
		t.Fatalf("admin request count mismatch\nGot:  %d\nWant: %d", g, w)
	}
	ddlReq, ok := reqs[0].(*databasepb.UpdateDatabaseDdlRequest)
	if !ok {
		t.Fatalf("unexpected admin request type %T", reqs[0])
	}
	if g, w := len(ddlReq.GetStatements()), 1; g != w {
		t.Fatalf("ddl statement count mismatch\nGot:  %d\nWant: %d", g, w)
	}
	gotDDL := ddlReq.GetStatements()[0]
	for _, wantFragment := range []string{
		`DEFAULT ('o\'reilly')`,
		`DEFAULT ('\\\' OR 1=1 --')`,
	} {
		if !strings.Contains(gotDDL, wantFragment) {
			t.Errorf("DDL default value mismatch\nGot:  %s\nWant: substring %s", gotDDL, wantFragment)
		}
	}

	// Verify DryRun works with Spanner array valuers and produces properly escaped explained SQL.
	dryDB := db.Session(&gorm.Session{DryRun: true, Logger: logger.Default.LogMode(logger.Silent)})
	res := dryDB.Table("singers").Where("first_name = ?", StringArray{"a'b", "c\\d"}).Find(&[]singer{})
	if res.Error != nil {
		t.Fatalf("DryRun query failed: %v", res.Error)
	}
	gotDrySQL := dryDB.Dialector.Explain(res.Statement.SQL.String(), res.Statement.Vars...)
	wantDrySQL := "SELECT * FROM `singers` WHERE first_name = ARRAY['a\\'b', 'c\\\\d'] AND `singers`.`deleted_at` IS NULL"
	if g, w := gotDrySQL, wantDrySQL; g != w {
		t.Errorf("DryRun explained SQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
}
