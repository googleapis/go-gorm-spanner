// Copyright 2023 Google LLC.
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
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"

	_ "github.com/googleapis/go-sql-spanner"
)

type Config struct {
	DriverName string

	// DSN is the Data Source Name that should be used to open a database connection.
	// Only set one of DSN, Connector, and Conn.
	DSN string

	// Connector is the driver.Connector that should be used to open a database connection.
	// Create a driver.Connector for Spanner by calling spannerdriver.CreateConnector.
	// A connector should be created only once and used to create all database connections.
	// Only set one of DSN, Connector, and Conn.
	Connector driver.Connector

	// Conn is a pre-created gorm connection pool.
	// Only set one of DSN, Connector, and Conn.
	Conn gorm.ConnPool

	// DisableAutoMigrateBatching turns off DDL batching for AutoMigrate calls.
	// Cloud Spanner by default uses DDL batching when AutoMigrate is called, as
	// executing multiple DDL statements in a single batch is a lot more efficient
	// than executing each statement separately. You should only use this option
	// if you are experiencing problems with the automatic batching of DDL
	// statements when calling AutoMigrate.
	DisableAutoMigrateBatching bool

	// DefaultSequenceKind is the value that will be used for auto-generated
	// primary keys. This configuration option defaults to 'bit_reversed_positive'
	// if no value has been set.
	//
	// Set this configuration option to AUTO_INCREMENT to use the AUTO_INCREMENT
	// keyword. The auto-generated value will then use the default_sequence_kind
	// that is configured in the database. This returns an error if the database
	// does not have a default_sequence_kind.
	//
	// Set this configuration option to DISABLED to fall back to using sequences
	// for auto-increment primary keys.
	DefaultSequenceKind string
}

type Dialector struct {
	*Config
}

func Open(dsn string) gorm.Dialector {
	return &Dialector{Config: &Config{DSN: dsn}}
}

func New(config Config) gorm.Dialector {
	return &Dialector{Config: &config}
}

func (dialector Dialector) Name() string {
	return "spanner"
}

func (dialector Dialector) Initialize(db *gorm.DB) (err error) {
	// Verify configuration.
	if dialector.Connector != nil && dialector.Conn != nil {
		return fmt.Errorf("only set one of Connector and Conn in the configuration")
	}
	if dialector.Connector != nil && dialector.DSN != "" {
		return fmt.Errorf("only set one of Connector and DSN in the configuration")
	}

	callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{
		CreateClauses: []string{"INSERT", "VALUES", "RETURNING"},
	})
	if dialector.DriverName == "" {
		dialector.DriverName = "spanner"
	}
	// Register an UPDATE callback that will ensure that primary key columns are
	// never included in the SET clause of the statement.
	updateCallback := db.Callback().Update()
	if err := updateCallback.
		After("gorm:before_update").
		Before("gorm:update").
		Register("gorm:spanner:remove_primary_key_from_update", BeforeUpdate); err != nil {
		return err
	}

	if dialector.Conn != nil {
		db.ConnPool = dialector.Conn
	} else if dialector.Connector != nil {
		db.ConnPool = sql.OpenDB(dialector.Connector)
	} else {
		db.ConnPool, err = sql.Open(dialector.DriverName, dialector.DSN)
		if err != nil {
			return err
		}
	}

	db.ClauseBuilders[clause.Insert{}.Name()] = insertHandler
	db.ClauseBuilders[clause.Returning{}.Name()] = func(c clause.Clause, builder clause.Builder) {
		builder.WriteString("THEN RETURN ")
		returning, ok := c.Expression.(clause.Returning)
		if ok && len(returning.Columns) > 0 {
			for idx, column := range returning.Columns {
				if idx > 0 {
					builder.WriteByte(',')
				}

				builder.WriteQuoted(column)
			}
		} else {
			builder.WriteByte('*')
		}
	}

	return
}

func insertHandler(c clause.Clause, builder clause.Builder) {
	insert, ok := c.Expression.(clause.Insert)
	if !ok {
		c.Build(builder)
		return
	}
	stmt, ok := builder.(*gorm.Statement)
	if !ok {
		c.Build(builder)
		return
	}
	onConflictClause, ok := stmt.Clauses[clause.OnConflict{}.Name()]
	if !ok {
		c.Build(builder)
		return
	}

	onConflict, ok := onConflictClause.Expression.(clause.OnConflict)
	if onConflict.OnConstraint != "" || onConflict.TargetWhere.Exprs != nil || onConflict.Where.Exprs != nil || !(onConflict.UpdateAll || onConflict.DoNothing) {
		_ = builder.AddError(fmt.Errorf("spanner only supports UpdateAll or DoNothing for OnConflict clauses"))
		return
	}

	if onConflict.UpdateAll {
		insert.Modifier = "INSERT OR UPDATE"
	} else if onConflict.DoNothing {
		insert.Modifier = "INSERT OR IGNORE"
	}
	insert.Build(builder)
}

func BeforeUpdate(db *gorm.DB) {
	// Omit all primary key fields from the SET clause of an UPDATE statement.
	db.Statement.Omit(db.Statement.Schema.PrimaryFieldDBNames...)
}

func (dialector Dialector) DefaultValueOf(field *schema.Field) clause.Expression {
	return clause.Expr{SQL: "NULL"}
}

func (dialector Dialector) Migrator(db *gorm.DB) gorm.Migrator {
	var conn *sql.Conn
	if c, ok := db.ConnPool.(*sql.Conn); ok && c != nil {
		conn = c
	} else {
		sqlDB, _ := db.DB()
		conn, _ = sqlDB.Conn(context.Background())
	}
	db.ConnPool = conn
	db.Statement.ConnPool = conn
	return spannerMigrator{
		Migrator: migrator.Migrator{
			Config: migrator.Config{
				DB:                          db,
				Dialector:                   dialector,
				CreateIndexAfterCreateTable: true,
			},
		},
		Dialector: dialector,
	}
}

func (dialector Dialector) BindVarTo(writer clause.Writer, stmt *gorm.Statement, v interface{}) {
	writer.WriteByte('?')
}

// needsGoogleSQLEscape reports whether character must be backslash-escaped inside a
// backtick-quoted GoogleSQL identifier.
func needsGoogleSQLEscape(character byte) bool {
	return character == '`' || character == '\\' || character < 0x20 || character == 0x7f
}

func isHexDigit(character byte) bool {
	return ('0' <= character && character <= '9') ||
		('a' <= character && character <= 'f') ||
		('A' <= character && character <= 'F')
}

func isOctalDigit(character byte) bool {
	return '0' <= character && character <= '7'
}

func allHexDigits(str string) bool {
	for index := 0; index < len(str); index++ {
		if !isHexDigit(str[index]) {
			return false
		}
	}
	return true
}

// isValidUnicodeEscapeHex reports whether hexDigits represents a valid Unicode scalar value
// in GoogleSQL (excluding UTF-16 surrogates 0xD800..0xDFFF and values > 0x10FFFF).
func isValidUnicodeEscapeHex(hexDigits string) bool {
	if !allHexDigits(hexDigits) {
		return false
	}
	codePoint, err := strconv.ParseUint(hexDigits, 16, 64)
	return err == nil && codePoint <= 0x10FFFF && !(0xD800 <= codePoint && codePoint <= 0xDFFF)
}

// validGoogleSQLEscapeLength returns the byte length of a valid GoogleSQL escape sequence
// starting at str[backslashIndex], ensuring at least one byte remains afterwards for the
// closing backtick.
func validGoogleSQLEscapeLength(str string, backslashIndex int) (int, bool) {
	if backslashIndex+2 >= len(str) {
		return 0, false
	}
	switch str[backslashIndex+1] {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', '?', '"', '\'', '`':
		// Single-character escapes: bell, backspace, form feed, newline, CR, tab, vertical tab, backslash, ?, quotes, backtick.
		return 2, true
	case 'x', 'X':
		// Two-digit hexadecimal byte escape: \xHH or \XHH.
		if backslashIndex+4 < len(str) && allHexDigits(str[backslashIndex+2:backslashIndex+4]) {
			return 4, true
		}
	case 'u':
		// Four-digit Unicode scalar value escape: \uHHHH (excluding UTF-16 surrogates).
		if backslashIndex+6 < len(str) && isValidUnicodeEscapeHex(str[backslashIndex+2:backslashIndex+6]) {
			return 6, true
		}
	case 'U':
		// Eight-digit Unicode scalar value escape: \UHHHHHHHH (<= 0x10FFFF, excluding UTF-16 surrogates).
		if backslashIndex+10 < len(str) && isValidUnicodeEscapeHex(str[backslashIndex+2:backslashIndex+10]) {
			return 10, true
		}
	case '0', '1', '2', '3':
		// Three-digit octal byte escape: \ooo (leading digit 0-3 ensures value <= 0377 / 255).
		if backslashIndex+4 < len(str) && isOctalDigit(str[backslashIndex+2]) && isOctalDigit(str[backslashIndex+3]) {
			return 4, true
		}
	}
	return 0, false
}

// writeEscapedByte writes character to writer, escaping backticks, backslashes,
// and ASCII control characters according to Cloud Spanner GoogleSQL rules.
func writeEscapedByte(writer clause.Writer, character byte) {
	switch character {
	case '`':
		writer.WriteString("\\`")
	case '\\':
		writer.WriteString("\\\\")
	case '\a':
		writer.WriteString("\\a")
	case '\b':
		writer.WriteString("\\b")
	case '\f':
		writer.WriteString("\\f")
	case '\n':
		writer.WriteString("\\n")
	case '\r':
		writer.WriteString("\\r")
	case '\t':
		writer.WriteString("\\t")
	case '\v':
		writer.WriteString("\\v")
	default:
		const hexDigits = "0123456789abcdef"
		writer.WriteString("\\x")
		writer.WriteByte(hexDigits[character>>4])
		writer.WriteByte(hexDigits[character&0x0f])
	}
}

// findSegmentEnd returns the end byte index of the dot-separated identifier segment
// starting at start, and whether the segment is already a validly backtick-quoted and
// escaped GoogleSQL identifier (non-empty, with no unescaped backticks, control characters,
// or invalid escape sequences).
func findSegmentEnd(str string, start int) (int, bool) {
	if str[start] == '`' {
		for index := start + 1; index < len(str); index++ {
			character := str[index]
			if character == '`' {
				if index > start+1 && (index+1 == len(str) || str[index+1] == '.') {
					return index + 1, true
				}
				break
			}
			if character == '\\' {
				escapeLen, ok := validGoogleSQLEscapeLength(str, index)
				if !ok {
					break
				}
				index += escapeLen - 1
				continue
			}
			if character < 0x20 || character == 0x7f {
				break
			}
		}
	}
	if dotIndex := strings.IndexByte(str[start:], '.'); dotIndex >= 0 {
		return start + dotIndex, false
	}
	return len(str), false
}

// writeQuotedIdentifierSegment writes a single identifier segment enclosed in backticks,
// escaping any internal backticks, backslashes, and control characters while flushing runs
// of unescaped bytes in bulk.
func writeQuotedIdentifierSegment(writer clause.Writer, segment string) {
	writer.WriteByte('`')
	lastFlushed := 0
	for index := 0; index < len(segment); index++ {
		if needsGoogleSQLEscape(segment[index]) {
			if index > lastFlushed {
				writer.WriteString(segment[lastFlushed:index])
			}
			writeEscapedByte(writer, segment[index])
			lastFlushed = index + 1
		}
	}
	if lastFlushed < len(segment) {
		writer.WriteString(segment[lastFlushed:])
	}
	writer.WriteByte('`')
}

// QuoteTo writes an identifier to writer quoted and escaped according to Cloud Spanner GoogleSQL rules.
// GoogleSQL uses backticks (`) for quoted identifiers. Internal backticks are escaped with a backslash (\`),
// backslashes are escaped as \\, and control characters use standard GoogleSQL escape sequences.
// Validly quoted and escaped segments are preserved idempotently without double-escaping.
func (dialector Dialector) QuoteTo(writer clause.Writer, str string) {
	if len(str) == 0 {
		// GoogleSQL rejects empty backtick identifiers (``), so emit nothing for empty inputs.
		return
	}
	if str == "*" {
		writer.WriteString("*")
		return
	}
	if strings.HasSuffix(str, ".*") {
		// Quote the schema/table prefix and preserve the trailing projection wildcard unquoted.
		dialector.QuoteTo(writer, str[:len(str)-2])
		writer.WriteString(".*")
		return
	}

	for index := 0; index < len(str); {
		if str[index] == '.' {
			writer.WriteByte('.')
			index++
			continue
		}

		segmentEnd, alreadyQuoted := findSegmentEnd(str, index)
		segment := str[index:segmentEnd]
		if alreadyQuoted {
			writer.WriteString(segment)
		} else {
			writeQuotedIdentifierSegment(writer, segment)
		}
		index = segmentEnd
	}
}

func (dialector Dialector) Explain(sql string, vars ...interface{}) string {
	return logger.ExplainSQL(sql, nil, `'`, vars...)
}

func (dialector Dialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case schema.Bool:
		return "BOOL"
	case schema.Int, schema.Uint:
		return "INT64"
	case schema.Float:
		if field.Size == 32 {
			return "FLOAT32"
		}
		return "FLOAT64"
	case schema.String:
		var size string
		if field.Size == 0 || field.Size > 2621440 {
			size = "MAX"
		} else {
			size = fmt.Sprintf("%d", field.Size)
		}
		return fmt.Sprintf("STRING(%s)", size)
	case schema.Bytes:
		var size string
		if field.Size == 0 || field.Size > 10485760 {
			size = "MAX"
		} else {
			size = fmt.Sprintf("%d", field.Size)
		}
		return fmt.Sprintf("BYTES(%s)", size)
	case schema.Time:
		return "TIMESTAMP"
	}

	return string(field.DataType)
}
