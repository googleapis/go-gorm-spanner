// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spannerpg

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"runtime"

	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

type Dialector struct {
	postgres.Dialector
	SpannerConfig SpannerConfig
}

// SpannerConfig contains custom configuration options for Spanner PostgreSQL.
type SpannerConfig struct {
	// DisableAutoMigrateBatching turns off DDL batching for AutoMigrate calls.
	// Spanner PostgreSQL by default uses DDL batching when AutoMigrate is called, as
	// executing multiple DDL statements in a single batch is a lot more efficient
	// than executing each statement separately. You should only use this option
	// if you are experiencing problems with the automatic batching of DDL
	// statements when calling AutoMigrate.
	DisableAutoMigrateBatching bool

	// AutoOrderByPk automatically adds an ORDER BY <pk> to all queries.
	// This flag is primarily intended for testing, as most gorm tests assume that queries will return query results
	// in primary key order, even when there is no ORDER BY clause in the query. Spanner does not guarantee this.
	AutoOrderByPk bool
	// AutoAddPrimaryKey automatically adds a 'generated_id serial' column to tables that are created without a
	// primary key. This flag is primarily intended for testing, as some gorm tests assumes that databases support
	// tables without a primary key. Spanner does not support this.
	AutoAddPrimaryKey bool
}

func Open(dsn string) gorm.Dialector {
	return &Dialector{
		Dialector: postgres.Dialector{Config: &postgres.Config{DSN: dsn}},
	}
}

func New(config postgres.Config) gorm.Dialector {
	return &Dialector{Dialector: postgres.Dialector{Config: &config}}
}

func NewWithSpannerConfig(config postgres.Config, spannerConfig SpannerConfig) gorm.Dialector {
	return &Dialector{Dialector: postgres.Dialector{Config: &config}, SpannerConfig: spannerConfig}
}

func (dialector Dialector) Name() string {
	return "postgres-spanner"
}

func (dialector Dialector) Initialize(db *gorm.DB) (err error) {
	if dialector.DriverName == "" {
		dialector.DriverName = "spanner"
	}

	if err := postgres.Dialector.Initialize(dialector.Dialector, db); err != nil {
		return err
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
	if dialector.SpannerConfig.AutoOrderByPk {
		queryCallback := db.Callback().Query()
		if err := queryCallback.
			Before("gorm:query").
			Register("gorm:spanner:auto_order_by", AutoOrderBy); err != nil {
			return err
		}
	}

	for k, v := range dialector.ClauseBuilders() {
		db.ClauseBuilders[k] = v
	}
	return nil
}

func AutoOrderBy(db *gorm.DB) {
	if db.DryRun {
		return
	}

	if db.Statement != nil && db.Statement.Schema != nil && db.Statement.Schema.PrimaryFields != nil {
		// Only add an ORDER BY clause for the top-level statement.
		if !isTopLevelStatement() {
			return
		}
		if db.Statement.Distinct {
			return
		}
		if selectClause, ok := db.Statement.Clauses["SELECT"]; ok {
			if expr, ok := selectClause.Expression.(clause.Expr); ok {
				if expr.SQL == "count(*)" {
					return
				}
			}
			if sel, ok := selectClause.Expression.(clause.Select); ok {
				if sel.Distinct {
					return
				}
			}
		}
		if _, ok := db.Statement.Clauses["GROUP BY"]; ok {
			return
		}

		for _, pk := range db.Statement.Schema.PrimaryFields {
			if pk.AutoIncrement {
				db.Statement.Order(fmt.Sprintf(`spanner.bit_reverse("%s"."%s", true)`, db.Statement.Table, pk.DBName))
			} else {
				db.Statement.Order(pk.DBName)
			}
		}
	}
}

func isTopLevelStatement() bool {
	callers := make([]uintptr, 100)
	length := runtime.Callers(0, callers)
	frames := runtime.CallersFrames(callers[0:length])
	count := 0
	for frame, more := frames.Next(); more; {
		if frame.Function == "gorm.io/gorm.(*processor).Execute" {
			count++
			if count > 1 {
				return false
			}
		}
		frame, more = frames.Next()
	}
	return true
}

func BeforeUpdate(db *gorm.DB) {
	if db.Statement != nil && db.Statement.Schema != nil && db.Statement.Schema.PrimaryFieldDBNames != nil {
		omits := make([]string, 0, len(db.Statement.Omits)+len(db.Statement.Schema.PrimaryFieldDBNames))
		omits = append(omits, db.Statement.Omits...)
		omits = append(omits, db.Statement.Schema.PrimaryFieldDBNames...)
		// Omit all primary key fields from the SET clause of an UPDATE statement.
		db.Statement.Omit(omits...)
	}
}

const (
	// ClauseOnConflict for clause.ClauseBuilder ON CONFLICT key
	ClauseOnConflict = "ON CONFLICT"
	// ClauseLimit for clause.ClauseBuilder LIMIT key
	ClauseLimit = "LIMIT"
)

func (dialector Dialector) ClauseBuilders() map[string]clause.ClauseBuilder {
	clauseBuilders := map[string]clause.ClauseBuilder{
		ClauseOnConflict: func(c clause.Clause, builder clause.Builder) {
			onConflict, ok := c.Expression.(clause.OnConflict)
			if !ok {
				c.Build(builder)
				return
			}
			if onConflict.DoNothing && len(onConflict.Columns) == 0 {
				if s := builder.(*gorm.Statement).Schema; s != nil {
					if len(s.PrimaryFields) == 0 {
						for _, field := range s.Fields {
							if field.DataType == "serial primary key" {
								onConflict.Columns = append(onConflict.Columns, clause.Column{Name: field.DBName})
							}
						}
					} else {
						for _, field := range s.PrimaryFields {
							onConflict.Columns = append(onConflict.Columns, clause.Column{Name: field.DBName})
						}
					}
					c.Expression = onConflict
				}
			}
			c.Build(builder)
		},
		ClauseLimit: func(c clause.Clause, builder clause.Builder) {
			if limit, ok := c.Expression.(clause.Limit); ok {
				if limit.Offset > 0 && limit.Limit == nil {
					l := math.MaxInt - limit.Offset
					limit.Limit = &l
					c.Expression = limit
				}
			}
			c.Build(builder)
		},
	}
	return clauseBuilders
}

func (dialector Dialector) Migrator(db *gorm.DB) gorm.Migrator {
	// Assign the migrator a single connection to ensure that batching works.
	var conn *sql.Conn
	if c, ok := db.ConnPool.(*sql.Conn); ok && c != nil {
		conn = c
	} else {
		sqlDB, err := db.DB()
		if err == nil {
			conn, _ = sqlDB.Conn(context.Background())
		}
	}
	// conn is nil if getting the underlying sql.DB failed.
	// In that case, we just use the original ConnPool of the database,
	// which again is likely to fail any operation on the migrator.
	if conn != nil {
		db.ConnPool = conn
		db.Statement.ConnPool = conn
	}
	return spannerPostgresMigrator{
		Migrator: postgres.Migrator{
			Migrator: migrator.Migrator{
				Config: migrator.Config{
					DB:                          db,
					Dialector:                   dialector,
					CreateIndexAfterCreateTable: true,
				},
			},
		},
		autoAddPrimaryKey: dialector.SpannerConfig.AutoAddPrimaryKey,
	}
}

func (dialector Dialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case schema.Int, schema.Uint:
		if field.AutoIncrement {
			return "serial"
		} else {
			return "int"
		}
	case schema.Float:
		return "numeric"
	case schema.Time:
		return "timestamptz"
	default:
		if field.AutoIncrement {
			return "serial"
		}
		return postgres.Dialector.DataTypeOf(dialector.Dialector, field)
	}
}

func getSerialDatabaseType(s string) (dbType string, ok bool) {
	switch s {
	case "smallserial":
		return "integer", true
	case "serial":
		return "integer", true
	case "bigserial":
		return "bigint", true
	default:
		return "", false
	}
}
