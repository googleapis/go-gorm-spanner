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
	"errors"
	"fmt"
	"sync"
	"testing"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	spannerparser "github.com/googleapis/go-sql-spanner/parser"
	"github.com/googleapis/go-sql-spanner/testutil"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var errQueryRecorded = errors.New("query recorded")

type recordingConnPool struct {
	mu      sync.Mutex
	queries []string
}

func (p *recordingConnPool) PrepareContext(_ context.Context, _ string) (*sql.Stmt, error) {
	return nil, errQueryRecorded
}

func (p *recordingConnPool) ExecContext(_ context.Context, query string, _ ...interface{}) (sql.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queries = append(p.queries, query)
	return nil, errQueryRecorded
}

func (p *recordingConnPool) QueryContext(_ context.Context, query string, _ ...interface{}) (*sql.Rows, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queries = append(p.queries, query)
	return nil, errQueryRecorded
}

func (p *recordingConnPool) QueryRowContext(_ context.Context, query string, _ ...interface{}) *sql.Row {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queries = append(p.queries, query)
	return nil
}

func (p *recordingConnPool) lastQuery() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queries) == 0 {
		return ""
	}
	return p.queries[len(p.queries)-1]
}

func setupRecordingAutoOrderByDB(t *testing.T, namer schema.Namer) (*gorm.DB, *recordingConnPool) {
	t.Helper()

	pool := &recordingConnPool{}
	dialector := NewWithSpannerConfig(postgres.Config{
		Conn: pool,
	}, SpannerConfig{
		AutoOrderByPk: true,
	})
	cfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}
	if namer != nil {
		cfg.NamingStrategy = namer
	}
	db, err := gorm.Open(dialector, cfg)
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}
	return db, pool
}

func executeRecordedQuery(t *testing.T, db *gorm.DB, pool *recordingConnPool, dest interface{}) string {
	t.Helper()

	err := db.Find(dest).Error
	if !errors.Is(err, errQueryRecorded) {
		t.Fatalf("unexpected query error\nGot:  %v\nWant: %v", err, errQueryRecorded)
	}
	gotSQL := pool.lastQuery()
	if gotSQL == "" {
		t.Fatal("expected query to be recorded, got empty string")
	}
	return gotSQL
}

type injectionTableAutoIncModel struct {
	ID   uint `gorm:"primarykey;autoIncrement"`
	Name string
}

func (injectionTableAutoIncModel) TableName() string {
	return `singers"); DROP TABLE users; --`
}

type customPKAutoIncModel struct {
	ID   uint `gorm:"primarykey;autoIncrement;column:my\"pk"`
	Name string
}

func (customPKAutoIncModel) TableName() string {
	return `my"table`
}

type sentinelNamesAutoIncModel struct {
	ID   uint `gorm:"primarykey;autoIncrement;column:~~~py~~~"`
	Name string
}

func (sentinelNamesAutoIncModel) TableName() string {
	return "~~~ct~~~"
}

type schemaQualifiedTableAutoIncModel struct {
	ID   uint `gorm:"primarykey;autoIncrement"`
	Name string
}

func (schemaQualifiedTableAutoIncModel) TableName() string {
	return "public.singers"
}

type injectionColumnNamer struct {
	schema.NamingStrategy
	columnName string
}

func (n injectionColumnNamer) ColumnName(table, column string) string {
	if column == "ID" {
		return n.columnName
	}
	return n.NamingStrategy.ColumnName(table, column)
}

func TestAutoOrderBy_AutoIncrement(t *testing.T) {
	t.Parallel()

	pgParser, err := spannerparser.NewStatementParser(databasepb.DatabaseDialect_POSTGRESQL, 0)
	if err != nil {
		t.Fatalf("failed to create statement parser: %v", err)
	}

	tests := []struct {
		name      string
		tableName string
		dest      func() interface{}
		wantSQL   string
	}{
		{
			name: "standard table and pk",
			dest: func() interface{} {
				type standardSinger struct {
					ID   uint `gorm:"primarykey;autoIncrement"`
					Name string
				}
				return &[]standardSinger{}
			},
			wantSQL: `SELECT * FROM "standard_singers" ORDER BY spanner.bit_reverse("standard_singers"."id", true)`,
		},
		{
			name:      "hyphenated table via db.Table",
			tableName: "my-singers",
			dest: func() interface{} {
				type row struct {
					ID   uint `gorm:"primarykey;autoIncrement"`
					Name string
				}
				return &[]row{}
			},
			wantSQL: `SELECT * FROM "my-singers" ORDER BY spanner.bit_reverse("my-singers"."id", true)`,
		},
		{
			name: "schema-qualified TableName",
			dest: func() interface{} {
				return &[]schemaQualifiedTableAutoIncModel{}
			},
			wantSQL: `SELECT * FROM "public"."singers" ORDER BY spanner.bit_reverse("singers"."id", true)`,
		},
		{
			name: "injection payload in TableName",
			dest: func() interface{} {
				return &[]injectionTableAutoIncModel{}
			},
			wantSQL: `SELECT * FROM "singers""); DROP TABLE users; --" ORDER BY spanner.bit_reverse("singers""); DROP TABLE users; --"."id", true)`,
		},
		{
			name: "double quotes in both table and column",
			dest: func() interface{} {
				return &[]customPKAutoIncModel{}
			},
			wantSQL: `SELECT * FROM "my""table" ORDER BY spanner.bit_reverse("my""table"."my""pk", true)`,
		},
		{
			name: "GORM sentinel names in table and pk",
			dest: func() interface{} {
				return &[]sentinelNamesAutoIncModel{}
			},
			wantSQL: `SELECT * FROM "~~~ct~~~" ORDER BY spanner.bit_reverse("~~~ct~~~"."~~~py~~~", true)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, pool := setupRecordingAutoOrderByDB(t, nil)
			queryDB := db
			if tc.tableName != "" {
				queryDB = db.Table(tc.tableName)
			}
			gotSQL := executeRecordedQuery(t, queryDB, pool, tc.dest())
			if g, w := gotSQL, tc.wantSQL; g != w {
				t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
			}

			hasMultiple, _, err := pgParser.Split(gotSQL)
			if err != nil {
				t.Fatalf("StatementParser.Split failed: %v", err)
			}
			if g, w := hasMultiple, false; g != w {
				t.Errorf("StatementParser.Split detected multiple statements\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestAutoOrderBy_SemicolonInjectionInPrimaryKeyName(t *testing.T) {
	t.Parallel()

	pgParser, err := spannerparser.NewStatementParser(databasepb.DatabaseDialect_POSTGRESQL, 0)
	if err != nil {
		t.Fatalf("failed to create statement parser: %v", err)
	}

	tests := []struct {
		name    string
		dest    func() interface{}
		wantSQL string
	}{
		{
			name: "auto increment pk",
			dest: func() interface{} {
				type singerWithAutoIncPK struct {
					ID   uint `gorm:"primarykey;autoIncrement"`
					Name string
				}
				return &[]singerWithAutoIncPK{}
			},
			wantSQL: `SELECT * FROM "singer_with_auto_inc_pks" ORDER BY spanner.bit_reverse("singer_with_auto_inc_pks"."pk""), true); DROP TABLE users; --", true)`,
		},
		{
			name: "non-auto increment pk",
			dest: func() interface{} {
				type singerWithManualPK struct {
					ID   string `gorm:"primarykey;autoIncrement:false"`
					Name string
				}
				return &[]singerWithManualPK{}
			},
			wantSQL: `SELECT * FROM "singer_with_manual_pks" ORDER BY "pk""), true); DROP TABLE users; --"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, pool := setupRecordingAutoOrderByDB(t, injectionColumnNamer{
				columnName: `pk"), true); DROP TABLE users; --`,
			})
			gotSQL := executeRecordedQuery(t, db, pool, tc.dest())
			if g, w := gotSQL, tc.wantSQL; g != w {
				t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
			}

			hasMultiple, _, err := pgParser.Split(gotSQL)
			if err != nil {
				t.Fatalf("StatementParser.Split failed: %v", err)
			}
			if g, w := hasMultiple, false; g != w {
				t.Errorf("StatementParser.Split detected multiple statements\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

type nonAutoIncInjectionModel struct {
	ID   string `gorm:"primarykey;autoIncrement:false;column:pk\"), true)"`
	Name string
}

func (nonAutoIncInjectionModel) TableName() string {
	return `my"singers`
}

func TestAutoOrderBy_NonAutoIncrement(t *testing.T) {
	t.Parallel()

	db, pool := setupRecordingAutoOrderByDB(t, nil)
	wantSQL := `SELECT * FROM "my""singers" ORDER BY "pk""), true)"`
	var results []nonAutoIncInjectionModel
	gotSQL := executeRecordedQuery(t, db, pool, &results)
	if g, w := gotSQL, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

type compositePKModel struct {
	ID       uint   `gorm:"primarykey;autoIncrement"`
	TenantID string `gorm:"primarykey;autoIncrement:false;column:tenant\"id"`
	Name     string
}

func (compositePKModel) TableName() string {
	return `my"items`
}

func TestAutoOrderBy_CompositePrimaryKey(t *testing.T) {
	t.Parallel()

	db, pool := setupRecordingAutoOrderByDB(t, nil)
	wantSQL := `SELECT * FROM "my""items" ORDER BY spanner.bit_reverse("my""items"."id", true),"tenant""id"`
	var results []compositePKModel
	gotSQL := executeRecordedQuery(t, db, pool, &results)
	if g, w := gotSQL, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestAutoOrderBy_CompositePrimaryKeyWithEmptyDBName(t *testing.T) {
	t.Parallel()

	db, _ := setupRecordingAutoOrderByDB(t, nil)
	stmt := &gorm.Statement{
		DB:      db,
		Clauses: map[string]clause.Clause{},
	}
	if err := stmt.Parse(&compositePKModel{}); err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}
	stmt.Schema.PrimaryFields[0].DBName = ""
	stmt.DB.Statement = stmt

	AutoOrderBy(stmt.DB)

	stmt.AddClauseIfNotExists(clause.Select{})
	stmt.AddClauseIfNotExists(clause.From{})
	stmt.Build("SELECT", "FROM", "ORDER BY")

	wantSQL := `SELECT * FROM "my""items" ORDER BY "tenant""id"`
	if g, w := stmt.SQL.String(), wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestAutoOrderBy_PreservesExistingOrder(t *testing.T) {
	t.Parallel()

	db, pool := setupRecordingAutoOrderByDB(t, nil)
	type singerRow struct {
		ID   uint `gorm:"primarykey;autoIncrement"`
		Name string
	}
	wantSQL := `SELECT * FROM "singer_rows" ORDER BY name DESC,spanner.bit_reverse("singer_rows"."id", true)`
	var results []singerRow
	gotSQL := executeRecordedQuery(t, db.Order("name DESC"), pool, &results)
	if g, w := gotSQL, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestAutoOrderBy_EmptyTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		model   interface{}
		wantSQL string
	}{
		{
			name: "auto increment primary key",
			model: &struct {
				ID uint `gorm:"primarykey;autoIncrement"`
			}{},
			wantSQL: `SELECT * FROM  ORDER BY spanner.bit_reverse("id", true)`,
		},
		{
			name: "non-auto increment primary key",
			model: &struct {
				ID string `gorm:"primarykey;autoIncrement:false"`
			}{},
			wantSQL: `SELECT * FROM  ORDER BY "id"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _ := setupRecordingAutoOrderByDB(t, nil)
			stmt := &gorm.Statement{
				DB:      db,
				Clauses: map[string]clause.Clause{},
			}
			if err := stmt.Parse(tc.model); err != nil {
				t.Fatalf("failed to parse schema: %v", err)
			}
			stmt.Table = ""
			stmt.DB.Statement = stmt

			AutoOrderBy(stmt.DB)

			stmt.AddClauseIfNotExists(clause.Select{})
			stmt.AddClauseIfNotExists(clause.From{})
			stmt.Build("SELECT", "FROM", "ORDER BY")

			if g, w := stmt.SQL.String(), tc.wantSQL; g != w {
				t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestAutoOrderBy_NoOrderByAdded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		model     interface{}
		clearPKDB bool
	}{
		{
			name: "empty primary key DBName",
			model: &struct {
				ID uint `gorm:"primarykey;autoIncrement"`
			}{},
			clearPKDB: true,
		},
		{
			name: "model without primary key",
			model: &struct {
				Name string
			}{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _ := setupRecordingAutoOrderByDB(t, nil)
			stmt := &gorm.Statement{
				DB:      db,
				Clauses: map[string]clause.Clause{},
			}
			if err := stmt.Parse(tc.model); err != nil {
				t.Fatalf("failed to parse schema: %v", err)
			}
			if tc.clearPKDB {
				stmt.Schema.PrimaryFields[0].DBName = ""
			}
			stmt.DB.Statement = stmt

			AutoOrderBy(stmt.DB)

			_, hasOrderBy := stmt.Clauses["ORDER BY"]
			if g, w := hasOrderBy, false; g != w {
				t.Errorf("unexpected ORDER BY clause presence\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestAutoOrderBy_DryRunSkipped(t *testing.T) {
	t.Parallel()

	db, _ := setupRecordingAutoOrderByDB(t, nil)
	type singerRow struct {
		ID uint `gorm:"primarykey;autoIncrement"`
	}
	var results []singerRow
	tx := db.Session(&gorm.Session{DryRun: true}).Find(&results)
	if tx.Error != nil {
		t.Fatalf("dry run query failed: %v", tx.Error)
	}
	wantSQL := `SELECT * FROM "singer_rows"`
	if g, w := tx.Statement.SQL.String(), wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestAutoOrderBy_MockServerEndToEnd(t *testing.T) {
	t.Parallel()

	server, _, serverTeardown := setupMockedTestServer(t)
	server.SetupSelectDialectResult(databasepb.DatabaseDialect_POSTGRESQL)
	dialector := NewWithSpannerConfig(postgres.Config{
		DSN: fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true;dialect=POSTGRESQL", server.Address),
	}, SpannerConfig{
		AutoOrderByPk: true,
	})
	db, server, teardown := setupTestGormConnectionWithDialector(t, server, serverTeardown, dialector)
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		teardown()
	}()

	wantSQL := `SELECT * FROM "singers""); DROP TABLE users; --" ORDER BY spanner.bit_reverse("singers""); DROP TABLE users; --"."id", true)`
	if err := server.TestSpanner.PutStatementResult(wantSQL, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "id"},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to put statement result: %v", err)
	}

	var results []injectionTableAutoIncModel
	if err := db.Find(&results).Error; err != nil {
		t.Fatalf("query failed: %v", err)
	}

	var gotSQL string
	for _, req := range server.TestSpanner.DrainRequestsFromServer() {
		if sqlReq, ok := req.(*spannerpb.ExecuteSqlRequest); ok {
			gotSQL = sqlReq.GetSql()
		}
	}
	if g, w := gotSQL, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

type JoinedDTOUser struct {
	ID   uint `gorm:"primarykey;autoIncrement"`
	Name string
}

type JoinedDTOLanguage struct {
	Code string `gorm:"primarykey;autoIncrement:false"`
	Name string
}

type JoinedDTOPet struct {
	ID     uint `gorm:"primarykey;autoIncrement"`
	UserID uint
}

func TestAutoOrderBy_JoinedMultiTableDTO(t *testing.T) {
	t.Parallel()

	type joinedResult struct {
		JoinedDTOUser
		JoinedDTOLanguage
		JoinedDTOPet
	}

	db, pool := setupRecordingAutoOrderByDB(t, nil)
	var results []joinedResult
	query := db.Select("users.*, languages.*, pets.*").
		Table("users").
		Joins("JOIN languages ON languages.code = users.language_code").
		Joins("LEFT OUTER JOIN pets ON pets.user_id = users.id")
	gotSQL := executeRecordedQuery(t, query, pool, &results)

	wantSQL := `SELECT users.*, languages.*, pets.* FROM "users" JOIN languages ON languages.code = users.language_code LEFT OUTER JOIN pets ON pets.user_id = users.id ORDER BY spanner.bit_reverse("users"."id", true),"code"`
	if g, w := gotSQL, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
	}
}

func TestAutoOrderBy_TableAliasAndSubquery(t *testing.T) {
	t.Parallel()

	type userRow struct {
		ID   uint `gorm:"primarykey;autoIncrement"`
		Name string
	}

	tests := []struct {
		name      string
		tableExpr string
		wantSQL   string
	}{
		{
			name:      "table with AS alias",
			tableExpr: "users AS u",
			wantSQL:   `SELECT * FROM users AS u ORDER BY spanner.bit_reverse("u"."id", true)`,
		},
		{
			name:      "subquery without alias falls back to model table name",
			tableExpr: "(SELECT * FROM users)",
			wantSQL:   `SELECT * FROM (SELECT * FROM users) ORDER BY spanner.bit_reverse("user_rows"."id", true)`,
		},
		{
			name:      "subquery with AS alias",
			tableExpr: "(SELECT * FROM users) AS u",
			wantSQL:   `SELECT * FROM (SELECT * FROM users) AS u ORDER BY spanner.bit_reverse("u"."id", true)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, pool := setupRecordingAutoOrderByDB(t, nil)
			var results []userRow
			gotSQL := executeRecordedQuery(t, db.Table(tc.tableExpr), pool, &results)
			if g, w := gotSQL, tc.wantSQL; g != w {
				t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestAutoOrderBy_FirstTakeLast(t *testing.T) {
	t.Parallel()

	type singerRow struct {
		ID   uint `gorm:"primarykey;autoIncrement"`
		Name string
	}

	tests := []struct {
		name    string
		run     func(db *gorm.DB, dest *singerRow) error
		wantSQL string
	}{
		{
			name: "First",
			run: func(db *gorm.DB, dest *singerRow) error {
				return db.First(dest).Error
			},
			wantSQL: `SELECT * FROM "singer_rows" ORDER BY "singer_rows"."id",spanner.bit_reverse("singer_rows"."id", true) LIMIT $1`,
		},
		{
			name: "Take",
			run: func(db *gorm.DB, dest *singerRow) error {
				return db.Take(dest).Error
			},
			wantSQL: `SELECT * FROM "singer_rows" ORDER BY spanner.bit_reverse("singer_rows"."id", true) LIMIT $1`,
		},
		{
			name: "Last",
			run: func(db *gorm.DB, dest *singerRow) error {
				return db.Last(dest).Error
			},
			wantSQL: `SELECT * FROM "singer_rows" ORDER BY "singer_rows"."id" DESC,spanner.bit_reverse("singer_rows"."id", true) LIMIT $1`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, pool := setupRecordingAutoOrderByDB(t, nil)
			var result singerRow
			err := tc.run(db, &result)
			if !errors.Is(err, errQueryRecorded) {
				t.Fatalf("unexpected query error\nGot:  %v\nWant: %v", err, errQueryRecorded)
			}
			if g, w := pool.lastQuery(), tc.wantSQL; g != w {
				t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}

func TestAutoOrderBy_SkippedQueryClauses(t *testing.T) {
	t.Parallel()

	type singerRow struct {
		ID   uint `gorm:"primarykey;autoIncrement"`
		Name string
	}

	tests := []struct {
		name    string
		run     func(db *gorm.DB) error
		wantSQL string
	}{
		{
			name: "Distinct via db.Distinct",
			run: func(db *gorm.DB) error {
				var results []singerRow
				return db.Distinct("name").Find(&results).Error
			},
			wantSQL: `SELECT DISTINCT "name" FROM "singer_rows"`,
		},
		{
			name: "Distinct via clause.Select",
			run: func(db *gorm.DB) error {
				var results []singerRow
				return db.Clauses(clause.Select{
					Distinct: true,
					Columns:  []clause.Column{{Name: "name"}},
				}).Find(&results).Error
			},
			wantSQL: `SELECT DISTINCT "name" FROM "singer_rows"`,
		},
		{
			name: "Count query",
			run: func(db *gorm.DB) error {
				var count int64
				return db.Model(&singerRow{}).Count(&count).Error
			},
			wantSQL: `SELECT count(*) FROM "singer_rows"`,
		},
		{
			name: "Group By query",
			run: func(db *gorm.DB) error {
				var results []singerRow
				return db.Select("name").Group("name").Find(&results).Error
			},
			wantSQL: `SELECT "name" FROM "singer_rows" GROUP BY "name"`,
		},
		{
			name: "nested subquery does not get AutoOrderBy",
			run: func(db *gorm.DB) error {
				var results []singerRow
				subQuery := db.Model(&singerRow{}).Select("id").Where("name = ?", "Alice")
				return db.Where("id IN (?)", subQuery).Find(&results).Error
			},
			wantSQL: `SELECT * FROM "singer_rows" WHERE id IN (SELECT "id" FROM "singer_rows" WHERE name = $1) ORDER BY spanner.bit_reverse("singer_rows"."id", true)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, pool := setupRecordingAutoOrderByDB(t, nil)
			err := tc.run(db)
			if !errors.Is(err, errQueryRecorded) {
				t.Fatalf("unexpected query error\nGot:  %v\nWant: %v", err, errQueryRecorded)
			}
			if g, w := pool.lastQuery(), tc.wantSQL; g != w {
				t.Errorf("SQL mismatch\nGot:  %s\nWant: %s", g, w)
			}
		})
	}
}
