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

func (dialector Dialector) QuoteTo(writer clause.Writer, str string) {
	var (
		underQuoted, selfQuoted bool
		continuousBacktick      int8
		shiftDelimiter          int8
	)

	for _, v := range []byte(str) {
		switch v {
		case '`':
			continuousBacktick++
			if continuousBacktick == 2 {
				writer.WriteString("``")
				continuousBacktick = 0
			}
		case '.':
			if continuousBacktick > 0 || !selfQuoted {
				shiftDelimiter = 0
				underQuoted = false
				continuousBacktick = 0
				writer.WriteString("`")
			}
			writer.WriteByte(v)
			continue
		default:
			if shiftDelimiter-continuousBacktick <= 0 && !underQuoted {
				writer.WriteByte('`')
				underQuoted = true
				if selfQuoted = continuousBacktick > 0; selfQuoted {
					continuousBacktick -= 1
				}
			}

			for ; continuousBacktick > 0; continuousBacktick -= 1 {
				writer.WriteString("``")
			}

			writer.WriteByte(v)
		}
		shiftDelimiter++
	}

	if continuousBacktick > 0 && !selfQuoted {
		writer.WriteString("``")
	}
	writer.WriteString("`")
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
