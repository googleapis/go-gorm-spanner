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
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"cloud.google.com/go/spanner"
	spannerdriver "github.com/googleapis/go-sql-spanner"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

type spannerPostgresMigrator struct {
	postgres.Migrator

	autoAddPrimaryKey bool
}

func (m spannerPostgresMigrator) queryRaw(sql string, values ...interface{}) (tx *gorm.DB) {
	queryTx := m.DB
	if m.DB.DryRun {
		queryTx = m.DB.Session(&gorm.Session{})
		queryTx.DryRun = false
	}
	return queryTx.Raw(sql, values...)
}

func (m spannerPostgresMigrator) queryRawWithTx(queryTx *gorm.DB, sql string, values ...interface{}) (tx *gorm.DB) {
	if queryTx.DryRun {
		queryTx = queryTx.Session(&gorm.Session{})
		queryTx.DryRun = false
	}
	return queryTx.Raw(sql, values...)
}

func (m spannerPostgresMigrator) CurrentDatabase() (name string) {
	// TODO: Extract from configuration
	return "db"
}

func (m spannerPostgresMigrator) CurrentSchema(stmt *gorm.Statement, table string) (interface{}, interface{}) {
	s, t := m.Migrator.CurrentSchema(stmt, table)
	if expr, ok := s.(clause.Expr); ok {
		if expr.SQL == "CURRENT_SCHEMA()" {
			return "public", t
		}
	}
	return s, t
}

func (m spannerPostgresMigrator) StartBatchDDL() error {
	return m.DB.Exec("START BATCH DDL").Error
}

func (m spannerPostgresMigrator) RunBatch() error {
	return m.DB.Exec("RUN BATCH").Error
}

func (m spannerPostgresMigrator) AbortBatch() error {
	return m.DB.Exec("ABORT BATCH").Error
}

func (m spannerPostgresMigrator) AutoMigrateDryRun(values ...interface{}) ([]spanner.Statement, error) {
	return m.autoMigrate( /* dryRun = */ true, values...)
}

func (m spannerPostgresMigrator) AutoMigrate(values ...interface{}) error {
	_, err := m.autoMigrate( /* dryRun = */ false, values...)
	return err
}

func (m spannerPostgresMigrator) disableAutoMigrateBatching() bool {
	if cfg, ok := m.Dialector.(Dialector); ok {
		return cfg.SpannerConfig.DisableAutoMigrateBatching
	}
	return false
}

func (m spannerPostgresMigrator) autoMigrate(dryRun bool, values ...interface{}) ([]spanner.Statement, error) {
	disableAutoBatching := m.disableAutoMigrateBatching()
	var c int64
	err := m.queryRaw("select count(1) from information_schema.database_options where schema_name='public' and option_name='default_sequence_kind'").Scan(&c).Error
	if err != nil {
		return nil, err
	}
	if dryRun || !disableAutoBatching {
		if err := m.StartBatchDDL(); err != nil {
			return nil, err
		}
		defer func() {
			// Abort any active batch when we leave this function.
			// This is a no-op if there is no batch on the current connection.
			_ = m.AbortBatch()
		}()
	}
	if c == 0 {
		tx := m.DB.Session(&gorm.Session{})
		// The database name is hardcoded in this string as "db", which might seem weird,
		// but Spanner ignores the database name in ALTER DATABASE statements.
		if err := tx.Exec(`alter database "db" set spanner.default_sequence_kind = 'bit_reversed_positive'`).Error; err != nil {
			return nil, err
		}
	}
	err = m.Migrator.AutoMigrate(values...)
	if err == nil {
		if !dryRun && disableAutoBatching {
			return nil, nil
		} else if dryRun {
			connPool := m.DB.Statement.ConnPool
			conn, ok := connPool.(*sql.Conn)
			if !ok {
				return nil, fmt.Errorf("unexpected ConnPool type")
			}
			var statements []spanner.Statement
			if err := conn.Raw(func(driverConn any) error {
				spannerConn, ok := driverConn.(spannerdriver.SpannerConn)
				if !ok {
					return fmt.Errorf("dry-run is only supported for Spanner")
				}
				statements = spannerConn.GetBatchedStatements()
				return nil
			}); err != nil {
				return nil, err
			}
			return statements, m.AbortBatch()
		} else {
			return nil, m.RunBatch()
		}
	}
	return nil, err
}

func (m spannerPostgresMigrator) GetTables() (tableList []string, err error) {
	currentSchema, _ := m.CurrentSchema(m.DB.Statement, "")
	return tableList, m.queryRaw("SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = ?", currentSchema, "BASE TABLE").Scan(&tableList).Error
}

func (m spannerPostgresMigrator) HasTable(value interface{}) bool {
	var count int64
	if err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentSchema, curTable := m.CurrentSchema(stmt, stmt.Table)
		return m.queryRaw("SELECT count(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ? AND table_type = ?", currentSchema, curTable, "BASE TABLE").Scan(&count).Error
	}); err != nil {
		return false
	}
	return count > 0
}

func (m spannerPostgresMigrator) CreateTable(values ...interface{}) (err error) {
	if !m.autoAddPrimaryKey {
		return m.Migrator.CreateTable(values...)
	}

	for _, value := range m.ReorderModels(values, false) {
		if err := m.RunWithValue(value, func(stmt *gorm.Statement) (err error) {
			if stmt.Schema == nil {
				// Let the base migrator handle this.
				return nil
			}
			var hasPrimaryKeyInDataType bool

			for _, dbName := range stmt.Schema.DBNames {
				field := stmt.Schema.FieldsByDBName[dbName]
				if !field.IgnoreMigration {
					hasPrimaryKeyInDataType = hasPrimaryKeyInDataType || strings.Contains(strings.ToUpper(m.DataTypeOf(field)), "PRIMARY KEY")
				}
			}

			// Add a generated primary key.
			if !hasPrimaryKeyInDataType && len(stmt.Schema.PrimaryFields) == 0 {
				pk := &schema.Field{
					Name:              "spanner_gorm_generated_id",
					DBName:            "spanner_gorm_generated_id",
					DataType:          "serial primary key",
					FieldType:         reflect.TypeOf(int64(0)),
					IndirectFieldType: reflect.TypeOf(int64(0)),
				}
				stmt.Schema.DBNames = append(stmt.Schema.DBNames, pk.DBName)
				stmt.Schema.Fields = append(stmt.Schema.Fields, pk)
				stmt.Schema.FieldsByDBName[pk.DBName] = pk
				stmt.Schema.FieldsByName[pk.Name] = pk
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return m.Migrator.CreateTable(values...)
}

func (m spannerPostgresMigrator) DropTable(values ...interface{}) error {
	values = m.ReorderModels(values, false)
	tx := m.DB.Session(&gorm.Session{})
	for i := len(values) - 1; i >= 0; i-- {
		if err := m.RunWithValue(values[i], func(stmt *gorm.Statement) error {
			if err := m.dropTableDependencies(tx, stmt); err != nil {
				return err
			}
			return tx.Exec("DROP TABLE IF EXISTS ?", m.CurrentTable(stmt)).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m spannerPostgresMigrator) dropTableDependencies(tx *gorm.DB, stmt *gorm.Statement) error {
	if err := m.dropForeignKeysReferencingTable(tx, stmt); err != nil {
		return err
	}
	if err := m.dropTableIndexes(tx, stmt); err != nil {
		return err
	}
	return nil
}

func (m spannerPostgresMigrator) dropForeignKeysReferencingTable(tx *gorm.DB, stmt *gorm.Statement) error {
	currentSchema, curTable := m.CurrentSchema(stmt, stmt.Table)
	fk := m.queryRawWithTx(tx, "select tc.table_name, ctu.constraint_name "+
		"from information_schema.constraint_table_usage ctu "+
		"inner join information_schema.table_constraints tc using (constraint_catalog, constraint_schema, constraint_name)"+
		"WHERE ctu.table_schema = ? AND ctu.table_name = ? AND tc.constraint_type in ('FOREIGN KEY')", currentSchema, curTable)
	if fk.Error != nil {
		return fk.Error
	}
	rows, err := fk.Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var tableName, constraintName string
		if err := rows.Scan(&tableName, &constraintName); err != nil {
			return err
		}
		if err := m.DropConstraint(tableName, constraintName); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

func (m spannerPostgresMigrator) dropTableIndexes(tx *gorm.DB, stmt *gorm.Statement) error {
	currentSchema, curTable := m.CurrentSchema(stmt, stmt.Table)
	idx := m.queryRawWithTx(tx, "select index_name from information_schema.indexes WHERE table_schema = ? AND table_name = ? AND index_type in ('INDEX') AND spanner_is_managed='NO'", currentSchema, curTable)
	if idx.Error != nil {
		return idx.Error
	}
	rows, err := idx.Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if err := m.DropIndex(curTable, name); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

func (m spannerPostgresMigrator) HasIndex(value interface{}, name string) bool {
	var count int64
	if err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if idx := stmt.Schema.LookIndex(name); idx != nil {
				name = idx.Name
			}
		}
		currentSchema, curTable := m.CurrentSchema(stmt, stmt.Table)
		return m.queryRaw(
			"SELECT count(*) FROM information_schema.indexes WHERE table_name = ? AND index_name = ? AND table_schema = ?", curTable, name, currentSchema,
		).Scan(&count).Error
	}); err != nil {
		return false
	}

	return count > 0
}

func (m spannerPostgresMigrator) HasColumn(value interface{}, field string) bool {
	var count int64
	if err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		name := field
		if stmt.Schema != nil {
			if field := stmt.Schema.LookUpField(field); field != nil {
				name = field.DBName
			}
		}

		currentSchema, curTable := m.CurrentSchema(stmt, stmt.Table)
		return m.queryRaw(
			"SELECT count(*) FROM INFORMATION_SCHEMA.columns WHERE table_schema = ? AND table_name = ? AND column_name = ?",
			currentSchema, curTable, name,
		).Scan(&count).Error
	}); err != nil {
		return false
	}

	return count > 0
}

func (m spannerPostgresMigrator) ColumnTypes(value interface{}) (columnTypes []gorm.ColumnType, err error) {
	columnTypes = make([]gorm.ColumnType, 0)
	err = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		var (
			currentSchema, table = m.CurrentSchema(stmt, stmt.Table)
			columns, err         = m.queryRaw(
				"SELECT c.column_name, c.is_nullable = 'YES', c.udt_name, c.character_maximum_length, c.numeric_precision, c.numeric_precision_radix, c.numeric_scale, c.datetime_precision, 8 * typlen, c.column_default, pd.description, c.identity_increment "+
					"FROM information_schema.columns AS c "+
					"LEFT JOIN pg_catalog.pg_type AS pgt ON c.udt_name = pgt.typname "+
					"LEFT JOIN pg_catalog.pg_description as pd ON pd.objsubid = c.ordinal_position AND pd.objoid = (SELECT oid FROM pg_catalog.pg_class WHERE relname = c.table_name AND relnamespace = (SELECT oid FROM pg_catalog.pg_namespace WHERE nspname = c.table_schema)) "+
					"where table_schema = ? AND table_name = ? and not column_name='spanner_gorm_generated_id'",
				currentSchema, table).Rows()
		)

		if err != nil {
			return err
		}

		for columns.Next() {
			var (
				column = &migrator.ColumnType{
					PrimaryKeyValue: sql.NullBool{Valid: true},
					UniqueValue:     sql.NullBool{Valid: true},
				}
				datetimePrecision sql.NullInt64
				radixValue        sql.NullInt64
				typeLenValue      sql.NullInt64
				identityIncrement sql.NullString
			)

			err = columns.Scan(
				&column.NameValue, &column.NullableValue, &column.DataTypeValue, &column.LengthValue, &column.DecimalSizeValue,
				&radixValue, &column.ScaleValue, &datetimePrecision, &typeLenValue, &column.DefaultValueValue, &column.CommentValue, &identityIncrement,
			)
			if err != nil {
				return err
			}

			if typeLenValue.Valid && typeLenValue.Int64 > 0 {
				column.LengthValue = typeLenValue
			}

			autoIncrementValuePattern := regexp.MustCompile(`^nextval\('"?[^']+seq"?'::regclass\)$`)
			if autoIncrementValuePattern.MatchString(column.DefaultValueValue.String) || (identityIncrement.Valid && identityIncrement.String != "") {
				column.AutoIncrementValue = sql.NullBool{Bool: true, Valid: true}
				column.DefaultValueValue = sql.NullString{}
			}

			if column.DefaultValueValue.Valid {
				column.DefaultValueValue.String = parseDefaultValueValue(column.DefaultValueValue.String)
			}

			if datetimePrecision.Valid {
				column.DecimalSizeValue = datetimePrecision
			}

			columnTypes = append(columnTypes, column)
		}
		if err := columns.Err(); err != nil {
			return err
		}
		if err := columns.Close(); err != nil {
			return err
		}

		// assign sql column type
		{
			rows, rowsErr := m.GetRows(currentSchema, table)
			if rowsErr != nil {
				return rowsErr
			}
			rawColumnTypes, err := rows.ColumnTypes()
			if err != nil {
				return err
			}
			for _, columnType := range columnTypes {
				for _, c := range rawColumnTypes {
					if c.Name() == columnType.Name() {
						columnType.(*migrator.ColumnType).SQLColumnType = c
						break
					}
				}
			}
			if err := rows.Close(); err != nil {
				return err
			}
		}

		// check primary, unique field
		{
			columnTypeRows, err := m.queryRaw("SELECT constraint_name FROM information_schema.table_constraints tc JOIN information_schema.constraint_column_usage AS ccu USING (constraint_schema, constraint_catalog, table_name, constraint_name) JOIN information_schema.columns AS c ON c.table_schema = tc.constraint_schema AND tc.table_name = c.table_name AND ccu.column_name = c.column_name WHERE constraint_type IN ('PRIMARY KEY', 'UNIQUE') AND c.table_schema = ? AND c.table_name = ? AND constraint_type = ? and not c.column_name='spanner_gorm_generated_id'", currentSchema, table, "UNIQUE").Rows()
			if err != nil {
				return err
			}
			uniqueContraints := map[string]int{}
			for columnTypeRows.Next() {
				var constraintName string
				if err := columnTypeRows.Scan(&constraintName); err != nil {
					return err
				}
				uniqueContraints[constraintName]++
			}
			if err := columnTypeRows.Err(); err != nil {
				return err
			}
			if err := columnTypeRows.Close(); err != nil {
				return err
			}

			columnTypeRows, err = m.queryRaw("SELECT c.column_name, constraint_name, constraint_type FROM information_schema.table_constraints tc JOIN information_schema.constraint_column_usage AS ccu USING (constraint_schema, constraint_catalog, table_name, constraint_name) JOIN information_schema.columns AS c ON c.table_schema = tc.constraint_schema AND tc.table_name = c.table_name AND ccu.column_name = c.column_name WHERE constraint_type IN ('PRIMARY KEY', 'UNIQUE') AND c.table_schema = ? AND c.table_name = ? and not c.column_name='spanner_gorm_generated_id'", currentSchema, table).Rows()
			if err != nil {
				return err
			}
			for columnTypeRows.Next() {
				var name, constraintName, columnType string
				if err := columnTypeRows.Scan(&name, &constraintName, &columnType); err != nil {
					return err
				}
				for _, c := range columnTypes {
					mc := c.(*migrator.ColumnType)
					if mc.NameValue.String == name {
						switch columnType {
						case "PRIMARY KEY":
							mc.PrimaryKeyValue = sql.NullBool{Bool: true, Valid: true}
						case "UNIQUE":
							if uniqueContraints[constraintName] == 1 {
								mc.UniqueValue = sql.NullBool{Bool: true, Valid: true}
							}
						}
						break
					}
				}
			}
			if err := columnTypeRows.Err(); err != nil {
				return err
			}
			if err := columnTypeRows.Close(); err != nil {
				return err
			}
		}

		// check column type
		{
			dataTypeRows, err := m.queryRaw(`select column_name, spanner_type as data_type
                                                 from information_schema.columns
                                                 where   table_schema=?
                                                 and     table_name=?
                                                 and not column_name='spanner_gorm_generated_id'`, currentSchema, table).Rows()
			if err != nil {
				return err
			}

			for dataTypeRows.Next() {
				var name, dataType string
				if err := dataTypeRows.Scan(&name, &dataType); err != nil {
					return err
				}
				for _, c := range columnTypes {
					mc := c.(*migrator.ColumnType)
					if mc.NameValue.String == name {
						mc.ColumnTypeValue = sql.NullString{String: dataType, Valid: true}
						// Handle array type: _text -> text[] , _int4 -> integer[]
						// Not support array size limits and array size limits because:
						// https://www.postgresql.org/docs/current/arrays.html#ARRAYS-DECLARATION
						if strings.HasPrefix(mc.DataTypeValue.String, "_") {
							mc.DataTypeValue = sql.NullString{String: dataType, Valid: true}
						}
						break
					}
				}
			}
			if err := dataTypeRows.Err(); err != nil {
				return err
			}
			if err := dataTypeRows.Close(); err != nil {
				return err
			}
		}

		return err
	})
	return
}

func (m spannerPostgresMigrator) MigrateColumn(value interface{}, field *schema.Field, columnType gorm.ColumnType) error {
	// Do not migrate primary key fields.
	if !field.PrimaryKey {
		// Skip the standard PostgreSQL migrator and go straight to the standard gorm migrator.
		if err := m.Migrator.Migrator.MigrateColumn(value, field, columnType); err != nil {
			return err
		}
	}
	return nil
}

func (m spannerPostgresMigrator) AlterColumn(value interface{}, field string) error {
	// Do not automatically modify generated columns.
	if m.isColumnGenerated(value, field) {
		return nil
	}
	err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if field := stmt.Schema.LookUpField(field); field != nil {
				var (
					columnTypes, _  = m.DB.Migrator().ColumnTypes(value)
					fieldColumnType *migrator.ColumnType
				)
				for _, columnType := range columnTypes {
					if columnType.Name() == field.DBName {
						fieldColumnType, _ = columnType.(*migrator.ColumnType)
					}
				}
				if fieldColumnType == nil {
					return fmt.Errorf("column type not found: %s", field.DBName)
				}

				fileType := clause.Expr{SQL: m.DataTypeOf(field)}
				// check for typeName and SQL name
				isSameType := true
				if !strings.EqualFold(fieldColumnType.DatabaseTypeName(), fileType.SQL) {
					isSameType = false
					// if different, also check for aliases
					aliases := m.GetTypeAliases(fieldColumnType.DatabaseTypeName())
					for _, alias := range aliases {
						if strings.HasPrefix(fileType.SQL, alias) {
							isSameType = true
							break
						}
					}
				}

				// not same, migrate
				if !isSameType {
					filedColumnAutoIncrement, _ := fieldColumnType.AutoIncrement()
					if field.AutoIncrement && filedColumnAutoIncrement { // update
						serialDatabaseType, _ := getSerialDatabaseType(fileType.SQL)
						if t, _ := fieldColumnType.ColumnType(); t != serialDatabaseType {
							if err := m.UpdateSequence(m.DB, stmt, field, serialDatabaseType); err != nil {
								return err
							}
						}
					} else if field.AutoIncrement && !filedColumnAutoIncrement { // create
						serialDatabaseType, _ := getSerialDatabaseType(fileType.SQL)
						if err := m.CreateSequence(m.DB, stmt, field, serialDatabaseType); err != nil {
							return err
						}
					} else if !field.AutoIncrement && filedColumnAutoIncrement { // delete
						if err := m.DeleteSequence(m.DB, stmt, field, fileType); err != nil {
							return err
						}
					} else {
						if err := m.modifyColumn(stmt, field, fileType, fieldColumnType); err != nil {
							return err
						}
					}
				}

				if null, _ := fieldColumnType.Nullable(); null == field.NotNull {
					if field.NotNull {
						if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? SET NOT NULL", m.CurrentTable(stmt), clause.Column{Name: field.DBName}).Error; err != nil {
							return err
						}
					} else {
						if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? DROP NOT NULL", m.CurrentTable(stmt), clause.Column{Name: field.DBName}).Error; err != nil {
							return err
						}
					}
				}

				if v, ok := fieldColumnType.DefaultValue(); (field.DefaultValueInterface == nil && ok) || v != field.DefaultValue {
					if field.HasDefaultValue && (field.DefaultValueInterface != nil || field.DefaultValue != "") {
						if field.DefaultValueInterface != nil {
							defaultStmt := &gorm.Statement{Vars: []interface{}{field.DefaultValueInterface}}
							m.Dialector.BindVarTo(defaultStmt, defaultStmt, field.DefaultValueInterface)
							if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? SET DEFAULT ?", m.CurrentTable(stmt), clause.Column{Name: field.DBName}, clause.Expr{SQL: m.Dialector.Explain(defaultStmt.SQL.String(), field.DefaultValueInterface)}).Error; err != nil {
								return err
							}
						} else if field.DefaultValue != "(-)" {
							if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? SET DEFAULT ?", m.CurrentTable(stmt), clause.Column{Name: field.DBName}, clause.Expr{SQL: field.DefaultValue}).Error; err != nil {
								return err
							}
						} else {
							if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? DROP DEFAULT", m.CurrentTable(stmt), clause.Column{Name: field.DBName}).Error; err != nil {
								return err
							}
						}
					} else if !field.HasDefaultValue {
						// case - as-is column has default value and to-be column has no default value
						// need to drop default
						if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? DROP DEFAULT", m.CurrentTable(stmt), clause.Column{Name: field.DBName}).Error; err != nil {
							return err
						}
					}
				}
				return nil
			}
		}
		return fmt.Errorf("failed to look up field with name: %s", field)
	})

	if err != nil {
		return err
	}
	return nil
}

func (m spannerPostgresMigrator) modifyColumn(stmt *gorm.Statement, field *schema.Field, targetType clause.Expr, existingColumn *migrator.ColumnType) error {
	alterSQL := "ALTER TABLE ? ALTER COLUMN ? TYPE ?"
	isUncastableDefaultValue := false

	if targetType.SQL == "boolean" {
		switch existingColumn.DatabaseTypeName() {
		case "int2", "int8", "numeric":
			alterSQL = "ALTER TABLE ? ALTER COLUMN ? TYPE ? USING ?::int::?"
		}
		isUncastableDefaultValue = true
	}

	if dv, _ := existingColumn.DefaultValue(); dv != "" && isUncastableDefaultValue {
		if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? DROP DEFAULT", m.CurrentTable(stmt), clause.Column{Name: field.DBName}).Error; err != nil {
			return err
		}
	}
	if isUncastableDefaultValue {
		if err := m.DB.Exec(alterSQL, m.CurrentTable(stmt), clause.Column{Name: field.DBName}, targetType, clause.Column{Name: field.DBName}, targetType).Error; err != nil {
			return err
		}
	} else {
		if err := m.DB.Exec(alterSQL, m.CurrentTable(stmt), clause.Column{Name: field.DBName}, targetType).Error; err != nil {
			return err
		}
	}
	return nil
}

func (m spannerPostgresMigrator) isColumnGenerated(value interface{}, field string) bool {
	var count int64
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentSchema := "public"
		name := field
		if field := stmt.Schema.LookUpField(field); field != nil {
			name = field.DBName
		}

		return m.DB.Raw(
			"SELECT count(*) FROM INFORMATION_SCHEMA.columns WHERE table_schema = ? AND table_name = ? AND column_name = ? AND generation_expression IS NOT NULL",
			currentSchema, stmt.Table, name,
		).Row().Scan(&count)
	})

	return count > 0
}

func parseDefaultValueValue(defaultValue string) string {
	value := regexp.MustCompile(`^(.*?)(?:::.*)?$`).ReplaceAllString(defaultValue, "$1")
	return strings.Trim(value, "'")
}

func (m spannerPostgresMigrator) HasConstraint(value interface{}, name string) bool {
	var count int64
	if err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		constraint, table := m.GuessConstraintInterfaceAndTable(stmt, name)
		if constraint != nil {
			name = constraint.GetName()
		}
		currentSchema, curTable := m.CurrentSchema(stmt, table)

		return m.queryRaw(
			"SELECT count(*) FROM INFORMATION_SCHEMA.table_constraints WHERE table_schema = ? AND table_name = ? AND constraint_name = ?",
			currentSchema, curTable, name,
		).Scan(&count).Error
	}); err != nil {
		return false
	}

	return count > 0
}

//const indexSql = `
//SELECT
//	ct.relname AS table_name,
//	ci.relname AS index_name,
//	i.indisunique AS non_unique,
//	i.indisprimary AS primary,
//	a.attname AS column_name
//FROM
//	pg_index i
//	LEFT JOIN pg_class ct ON ct.oid = i.indrelid
//	LEFT JOIN pg_class ci ON ci.oid = i.indexrelid
//	LEFT JOIN pg_attribute a ON a.attrelid = ct.oid
//	LEFT JOIN pg_constraint con ON con.conindid = i.indexrelid
//WHERE
//	a.attnum = ANY(i.indkey)
//	AND con.oid IS NULL
//	AND ct.relkind = 'r'
//	AND ct.relname = ?
//`

const indexSql = `
SELECT
	i.table_name AS table_name,
	i.index_name AS index_name,
	case when i.is_unique = 'YES' then true else false end as non_unique,
	case when i.index_type = 'PRIMARY_KEY' then true else false end as primary,
	ic.column_name AS column_name
FROM
    information_schema.indexes i
INNER JOIN
	information_schema.index_columns ic using (table_catalog, table_schema, table_name, index_name)
WHERE
    ic.ordinal_position is not null
AND i.spanner_is_managed = 'NO'
AND i.table_name = ?
ORDER BY i.table_schema, i.table_name, i.index_name, ic.ordinal_position
`

func (m spannerPostgresMigrator) GetIndexes(value interface{}) ([]gorm.Index, error) {
	indexes := make([]gorm.Index, 0)

	err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		result := make([]*postgres.Index, 0)
		scanErr := m.queryRaw(indexSql, stmt.Table).Scan(&result).Error
		if scanErr != nil {
			return scanErr
		}
		indexMap := groupByIndexName(result)
		for _, idx := range indexMap {
			tempIdx := &migrator.Index{
				TableName: idx[0].TableName,
				NameValue: idx[0].IndexName,
				PrimaryKeyValue: sql.NullBool{
					Bool:  idx[0].Primary,
					Valid: true,
				},
				UniqueValue: sql.NullBool{
					Bool:  idx[0].NonUnique,
					Valid: true,
				},
			}
			for _, x := range idx {
				tempIdx.ColumnList = append(tempIdx.ColumnList, x.ColumnName)
			}
			indexes = append(indexes, tempIdx)
		}
		return nil
	})
	return indexes, err
}

func groupByIndexName(indexList []*postgres.Index) map[string][]*postgres.Index {
	columnIndexMap := make(map[string][]*postgres.Index, len(indexList))
	for _, idx := range indexList {
		columnIndexMap[idx.IndexName] = append(columnIndexMap[idx.IndexName], idx)
	}
	return columnIndexMap
}
