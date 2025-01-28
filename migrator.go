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
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"cloud.google.com/go/spanner"
	spannerdriver "github.com/googleapis/go-sql-spanner"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

const (
	gormSpannerSequenceTag = "gorm_sequence_name"
)

type SpannerMigrator interface {
	gorm.Migrator

	AutoMigrateDryRun(values ...interface{}) ([]spanner.Statement, error)
	StartBatchDDL() error
	RunBatch() error
	AbortBatch() error
}

type spannerMigrator struct {
	migrator.Migrator
	Dialector
	dryRun bool
}

type spannerColumnType struct {
	migrator.ColumnType
	GenerationExpression sql.NullString
}

type Index struct {
	TableName    string
	ColumnName   string
	IndexName    string
	IsUnique     sql.NullBool
	IsPrimaryKey sql.NullBool
}

func (m spannerMigrator) CurrentDatabase() (name string) {
	return ""
}

func (m spannerMigrator) AutoMigrateDryRun(values ...interface{}) ([]spanner.Statement, error) {
	return m.autoMigrate( /* dryRun = */ true, values...)
}

func (m spannerMigrator) AutoMigrate(values ...interface{}) error {
	_, err := m.autoMigrate( /* dryRun = */ false, values...)
	return err
}

func (m spannerMigrator) autoMigrate(dryRun bool, values ...interface{}) ([]spanner.Statement, error) {
	if dryRun || !m.Dialector.Config.DisableAutoMigrateBatching {
		if err := m.StartBatchDDL(); err != nil {
			return nil, err
		}
	}
	err := m.Migrator.AutoMigrate(values...)
	if err == nil {
		if !dryRun && m.Dialector.Config.DisableAutoMigrateBatching {
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

func (m spannerMigrator) StartBatchDDL() error {
	return m.DB.Exec("START BATCH DDL").Error
}

func (m spannerMigrator) RunBatch() error {
	return m.DB.Exec("RUN BATCH").Error
}

func (m spannerMigrator) AbortBatch() error {
	return m.DB.Exec("ABORT BATCH").Error
}

// FullDataTypeOf returns field's db full data type
func (m spannerMigrator) FullDataTypeOf(field *schema.Field) (expr clause.Expr) {
	expr.SQL = m.Migrator.DataTypeOf(field)

	if field.NotNull {
		expr.SQL += " NOT NULL"
	}

	if field.HasDefaultValue && (field.DefaultValueInterface != nil || field.DefaultValue != "") {
		if field.DefaultValueInterface != nil {
			defaultStmt := &gorm.Statement{Vars: []interface{}{field.DefaultValueInterface}}
			m.Dialector.BindVarTo(defaultStmt, defaultStmt, field.DefaultValueInterface)
			expr.SQL += " DEFAULT (" + m.Dialector.Explain(defaultStmt.SQL.String(), field.DefaultValueInterface) + ")"
		} else if field.DefaultValue != "(-)" {
			expr.SQL += " DEFAULT (" + field.DefaultValue + ")"
		}
	}

	return
}

func (m spannerMigrator) CreateTable(values ...interface{}) error {
	for _, value := range m.ReorderModels(values, false) {
		tx := m.DB.Session(&gorm.Session{})
		if err := m.RunWithValue(value, func(stmt *gorm.Statement) (errr error) {
			var (
				createTableSQL          = "CREATE TABLE ? ("
				values                  = []interface{}{m.CurrentTable(stmt)}
				hasPrimaryKeyInDataType bool
			)
			for _, f := range stmt.Schema.Fields {
				// Cloud spanner does not support auto incrementing primary keys.
				if f.AutoIncrement && f.HasDefaultValue && f.DefaultValue == "" && f.DefaultValueInterface == nil {
					sequence := f.Tag.Get(gormSpannerSequenceTag)
					if sequence == "" {
						sequence = stmt.Table + "_seq"
					}
					if err := tx.Exec("CREATE SEQUENCE IF NOT EXISTS " +
						sequence +
						` OPTIONS (sequence_kind = "bit_reversed_positive")`).Error; err != nil {
						return err
					}
					f.DefaultValue = "GET_NEXT_SEQUENCE_VALUE(Sequence " + sequence + ")"
					// Reset the default value to nothing after finishing migration.
					//goland:noinspection GoDeferInLoop
					defer func() { f.DefaultValue = "" }()
				}
			}
			for _, dbName := range stmt.Schema.DBNames {
				field := stmt.Schema.FieldsByDBName[dbName]
				if !field.IgnoreMigration {
					createTableSQL += "? ?"
					hasPrimaryKeyInDataType = hasPrimaryKeyInDataType || strings.Contains(strings.ToUpper(string(field.DataType)), "PRIMARY KEY")
					values = append(values, clause.Column{Name: dbName}, m.DB.Migrator().FullDataTypeOf(field))
					createTableSQL += ","
				}
			}

			// Indexes should always be created after the table, as Spanner does not support
			// inline index creation. Iterate over the indexes in a fixed order to make the
			// script outcome deterministic.
			indexes := stmt.Schema.ParseIndexes()
			indexNames := make([]string, 0, len(indexes))
			for name := range indexes {
				indexNames = append(indexNames, name)
			}
			slices.Sort(indexNames)
			for _, name := range indexNames {
				idx := indexes[name]
				defer func(value interface{}, name string) {
					if errr == nil {
						errr = tx.Migrator().CreateIndex(value, name)
					}
				}(value, idx.Name)
			}

			// Iterator over the relationships in a fixed order.
			relationshipKeys := make([]string, 0, len(stmt.Schema.Relationships.Relations))
			for key := range stmt.Schema.Relationships.Relations {
				relationshipKeys = append(relationshipKeys, key)
			}
			slices.Sort(relationshipKeys)
			for _, key := range relationshipKeys {
				if !m.DB.DisableForeignKeyConstraintWhenMigrating {
					rel := stmt.Schema.Relationships.Relations[key]
					if constraint := rel.ParseConstraint(); constraint != nil {
						if constraint.Schema == stmt.Schema {
							sql, vars := buildConstraint(constraint)
							createTableSQL += sql + ","
							values = append(values, vars...)
						}
					}
				}
			}

			if len(stmt.Schema.ParseUniqueConstraints()) > 0 {
				return errUniqueConstraintNotSupported
			}
			for _, chk := range stmt.Schema.ParseCheckConstraints() {
				createTableSQL += "CONSTRAINT ? CHECK (?),"
				values = append(values, clause.Column{Name: chk.Name}, clause.Expr{SQL: chk.Constraint})
			}

			createTableSQL = strings.TrimSuffix(createTableSQL, ",")

			createTableSQL += ")"

			if !hasPrimaryKeyInDataType && len(stmt.Schema.PrimaryFields) > 0 {
				createTableSQL += " PRIMARY KEY ?"
				primaryKeys := []interface{}{}
				for _, field := range stmt.Schema.PrimaryFields {
					primaryKeys = append(primaryKeys, clause.Column{Name: field.DBName})
				}

				values = append(values, primaryKeys)
			}

			if tableOption, ok := m.DB.Get("gorm:table_options"); ok {
				createTableSQL += fmt.Sprint(tableOption)
			}

			errr = tx.Exec(createTableSQL, values...).Error
			return errr
		}); err != nil {
			return err
		}
	}
	return nil
}

// DropTable drop table for values
func (m spannerMigrator) DropTable(values ...interface{}) error {
	values = m.ReorderModels(values, false)
	for i := len(values) - 1; i >= 0; i-- {
		tx := m.DB.Session(&gorm.Session{})
		if err := m.RunWithValue(values[i], func(stmt *gorm.Statement) error {
			return tx.Exec("DROP TABLE ?", m.CurrentTable(stmt)).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m spannerMigrator) HasIndex(value interface{}, name string) bool {
	var count int64
	m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentDatabase := m.DB.Migrator().CurrentDatabase()
		if idx := stmt.Schema.LookIndex(name); idx != nil {
			name = idx.Name
		}

		return m.DB.Raw(
			"SELECT count(*) FROM information_schema.indexes WHERE table_schema = ? AND table_name = ? AND index_name = ?",
			currentDatabase, stmt.Table, name,
		).Row().Scan(&count)
	})

	return count > 0
}

func (m spannerMigrator) GetIndexes(value interface{}) ([]gorm.Index, error) {
	const indexSQL = `
	SELECT 
		i.index_name,
		i.is_unique,
		i.index_type = 'PRIMARY_KEY' as is_primary_key,
		i.index_type,
		col.column_name
	FROM
		information_schema.indexes i
		INNER JOIN information_schema.index_columns ic
		     ON ic.table_catalog = i.table_catalog
		    AND ic.table_schema =  i.table_schema
		    AND ic.table_name =    i.table_name
		    AND ic.index_name =    i.index_name
		INNER JOIN information_schema.columns col
		    ON  col.column_name = ic.column_name
		    AND col.table_name  = ic.table_name
	        AND col.table_schema = ic.table_schema
	        AND col.table_catalog = ic.table_catalog
	WHERE i.spanner_is_managed = false
	  AND i.table_schema = ?
	  AND i.table_name = ?
	ORDER BY i.table_catalog, i.table_schema, i.table_name, i.index_name, ic.ordinal_position
	`
	indexes := make([]gorm.Index, 0)
	err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentDatabase := m.DB.Migrator().CurrentDatabase()
		result := make([]*Index, 0)
		if err := m.DB.Raw(indexSQL, currentDatabase, stmt.Table).Scan(&result).Error; err != nil {
			return err
		}
		indexMap := make(map[string]*migrator.Index)
		for _, r := range result {
			idx, ok := indexMap[r.IndexName]
			if !ok {
				idx = &migrator.Index{
					TableName:       stmt.Table,
					NameValue:       r.IndexName,
					ColumnList:      nil,
					PrimaryKeyValue: r.IsPrimaryKey,
					UniqueValue:     r.IsUnique,
				}
			}
			idx.ColumnList = append(idx.ColumnList, r.ColumnName)
			indexMap[r.IndexName] = idx
		}
		for _, idx := range indexMap {
			indexes = append(indexes, idx)
		}
		sort.Slice(indexes, func(i, j int) bool {
			return indexes[i].Name() < indexes[j].Name()
		})
		return nil
	})
	return indexes, err
}

func (m spannerMigrator) DropIndex(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if idx := stmt.Schema.LookIndex(name); idx != nil {
			name = idx.Name
		}

		return m.DB.Exec("DROP INDEX ?", clause.Column{Name: name}).Error
	})
}

func (m spannerMigrator) AlterColumn(value interface{}, field string) error {
	// Do not automatically modify generated columns.
	if m.isColumnGenerated(value, field) {
		return nil
	}
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if field := stmt.Schema.LookUpField(field); field != nil {
			fullType := m.FullDataTypeOf(field)
			return m.DB.Exec(
				"ALTER TABLE ? ALTER COLUMN ? ?",
				m.CurrentTable(stmt), clause.Column{Name: field.DBName}, fullType,
			).Error
		}
		return fmt.Errorf("failed to look up field with name: %s", field)
	})
}

// ColumnTypes column types return columnTypes,error
func (m spannerMigrator) ColumnTypes(value interface{}) ([]gorm.ColumnType, error) {
	columnTypes := make([]gorm.ColumnType, 0)
	err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		columnTypeSQL := `
				SELECT COLUMN_NAME, COLUMN_DEFAULT, IS_NULLABLE = 'YES',
					   REGEXP_REPLACE(SPANNER_TYPE, '\\(.*\\)', '') AS DATA_TYPE,
					   SAFE_CAST(REPLACE(REPLACE(REGEXP_EXTRACT(SPANNER_TYPE, '\\(.*\\)'), '(', ''), ')', '') AS INT64) AS COLUMN_LENGTH,
					   (SELECT IF(I.INDEX_TYPE='PRIMARY_KEY', 'PRI', 'UNI')
						FROM INFORMATION_SCHEMA.INDEXES I
						INNER JOIN INFORMATION_SCHEMA.INDEX_COLUMNS IC USING (TABLE_CATALOG, TABLE_SCHEMA, TABLE_NAME, INDEX_NAME)
						WHERE IC.TABLE_CATALOG = C.TABLE_CATALOG
						  AND IC.TABLE_SCHEMA =  C.TABLE_SCHEMA
						  AND IC.TABLE_NAME =    C.TABLE_NAME
						  AND IC.COLUMN_NAME =   C.COLUMN_NAME
						  AND I.IS_UNIQUE
						ORDER BY I.INDEX_TYPE
						LIMIT 1
					   ) AS KEY,
                    `
		rows, err := m.DB.Session(&gorm.Session{}).Table(stmt.Table).Limit(1).Rows()
		if err != nil {
			return err
		}
		rawColumnTypes, err := rows.ColumnTypes()
		if err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}

		columnTypeSQL += "FROM INFORMATION_SCHEMA.COLUMNS C WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION"
		currentDatabase := m.CurrentDatabase()
		columns, rowErr := m.DB.Table(stmt.Table).Raw(columnTypeSQL, &currentDatabase, &stmt.Table).Rows()
		if rowErr != nil {
			return rowErr
		}

		defer columns.Close()

		for columns.Next() {
			var (
				column    migrator.ColumnType
				columnKey sql.NullString
				values    = []interface{}{
					&column.NameValue, &column.DefaultValueValue, &column.NullableValue, &column.DataTypeValue, &column.LengthValue, &columnKey,
				}
			)
			if scanErr := columns.Scan(values...); scanErr != nil {
				return scanErr
			}

			column.PrimaryKeyValue = sql.NullBool{Bool: false, Valid: true}
			column.UniqueValue = sql.NullBool{Bool: false, Valid: true}
			switch columnKey.String {
			case "PRI":
				column.PrimaryKeyValue = sql.NullBool{Bool: true, Valid: true}
			case "UNI":
				column.UniqueValue = sql.NullBool{Bool: true, Valid: true}
			}
			column.DefaultValueValue.String = strings.Trim(column.DefaultValueValue.String, "'")

			for _, c := range rawColumnTypes {
				if c.Name() == column.NameValue.String {
					column.SQLColumnType = c
					break
				}
			}
			if column.SQLColumnType == nil {
				column.SQLColumnType = &sql.ColumnType{}
			}

			columnTypes = append(columnTypes, column)
		}

		return nil
	})

	return columnTypes, err
}

func (m spannerMigrator) isColumnGenerated(value interface{}, field string) bool {
	var count int64
	m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentDatabase := m.DB.Migrator().CurrentDatabase()
		name := field
		if field := stmt.Schema.LookUpField(field); field != nil {
			name = field.DBName
		}

		return m.DB.Raw(
			"SELECT count(*) FROM INFORMATION_SCHEMA.columns WHERE table_schema = ? AND table_name = ? AND column_name = ? AND generation_expression IS NOT NULL",
			currentDatabase, stmt.Table, name,
		).Row().Scan(&count)
	})

	return count > 0
}

func buildConstraint(constraint *schema.Constraint) (sql string, results []interface{}) {
	sql = "CONSTRAINT ? FOREIGN KEY ? REFERENCES ??"
	if constraint.OnDelete != "" {
		sql += " ON DELETE " + constraint.OnDelete
	}

	if constraint.OnUpdate != "" {
		sql += " ON UPDATE " + constraint.OnUpdate
	}

	var foreignKeys, references []interface{}
	for _, field := range constraint.ForeignKeys {
		foreignKeys = append(foreignKeys, clause.Column{Name: field.DBName})
	}

	for _, field := range constraint.References {
		references = append(references, clause.Column{Name: field.DBName})
	}
	results = append(results, clause.Table{Name: constraint.Name}, foreignKeys, clause.Table{Name: constraint.ReferenceSchema.Table}, references)
	return
}

type Column struct {
	name     string
	nullable sql.NullString
	datatype string
	maxLen   sql.NullInt64
}

func (c Column) Name() string {
	return c.name
}

func (c Column) DatabaseTypeName() string {
	return c.datatype
}

func (c Column) Length() (int64, bool) {
	if c.maxLen.Valid {
		return c.maxLen.Int64, c.maxLen.Valid
	}

	return 0, false
}

func (c Column) Nullable() (bool, bool) {
	if c.nullable.Valid {
		return c.nullable.String == "YES", true
	}

	return false, false
}

// DecimalSize return precision int64, scale int64, ok bool
func (c Column) DecimalSize() (int64, int64, bool) {
	if c.datatype == "NUMERIC" {
		return 38, 9, true
	}
	return 0, 0, false
}

func (m spannerMigrator) isUniqueConstraint(value interface{}, name string) bool {
	isUnique := false
	if err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		uniqueConstraints := stmt.Schema.ParseUniqueConstraints()
		if _, ok := uniqueConstraints[name]; ok {
			isUnique = true
		}
		return nil
	}); err != nil {
		return false
	}
	return isUnique
}

var errUniqueConstraintNotSupported = errors.New("unique constraints are not supported by Spanner, use a unique index instead")

// CreateConstraint ignores requests to create unique constraints, as Spanner
// does not support them.
func (m spannerMigrator) CreateConstraint(value interface{}, name string) error {
	if m.isUniqueConstraint(value, name) {
		return errUniqueConstraintNotSupported
	}
	return m.Migrator.CreateConstraint(value, name)
}

// DropConstraint ignores requests to drop unique constraints, as Spanner
// does not support them.
func (m spannerMigrator) DropConstraint(value interface{}, name string) error {
	if m.isUniqueConstraint(value, name) {
		return errUniqueConstraintNotSupported
	}
	return m.Migrator.DropConstraint(value, name)
}
