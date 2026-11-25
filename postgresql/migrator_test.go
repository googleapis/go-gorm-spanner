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
	"fmt"
	"testing"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/go-sql-spanner/testutil"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type singer struct {
	gorm.Model
	FirstName string
	LastName  string
	FullName  string
	Active    bool
}

type album struct {
	gorm.Model
	Title    string
	Rating   float32
	SingerID uint
	Singer   *singer
}

type test struct {
	ID       uint
	Test     string
	SingerID uint
	Singer   *singer
}

func TestMigrate(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()
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

	err = db.Migrator().AutoMigrate(&singer{}, &album{}, &test{})
	if err != nil {
		t.Fatal(err)
	}
	requests := server.TestDatabaseAdmin.Reqs()
	if g, w := len(requests), 1; g != w {
		t.Fatalf("request count mismatch\n Got: %v\nWant: %v", g, w)
	}
	request := requests[0].(*databasepb.UpdateDatabaseDdlRequest)
	if g, w := len(request.GetStatements()), 5; g != w {
		t.Fatalf("statement count mismatch\n Got: %v\nWant: %v", g, w)
	}
	index := 0
	if g, w := request.GetStatements()[index], `CREATE TABLE "singers" ("id" serial,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"first_name" text,"last_name" text,"full_name" text,"active" boolean,PRIMARY KEY ("id"))`; g != w {
		t.Fatalf("create singers statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	index++
	if g, w := request.GetStatements()[index], `CREATE INDEX IF NOT EXISTS "idx_singers_deleted_at" ON "singers" ("deleted_at")`; g != w {
		t.Fatalf("create idx_singers_deleted_at statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	index++
	if g, w := request.GetStatements()[index], `CREATE TABLE "albums" ("id" serial,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"title" text,"rating" numeric,"singer_id" int,PRIMARY KEY ("id"),CONSTRAINT "fk_albums_singer" FOREIGN KEY ("singer_id") REFERENCES "singers"("id"))`; g != w {
		t.Fatalf("create albums statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	index++
	if g, w := request.GetStatements()[index], `CREATE INDEX IF NOT EXISTS "idx_albums_deleted_at" ON "albums" ("deleted_at")`; g != w {
		t.Fatalf("create idx_albums_deleted_at statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	index++
	if g, w := request.GetStatements()[index], `CREATE TABLE "tests" ("id" serial,"test" text,"singer_id" int,PRIMARY KEY ("id"),CONSTRAINT "fk_tests_singer" FOREIGN KEY ("singer_id") REFERENCES "singers"("id"))`; g != w {
		t.Fatalf("create albums statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
}

func TestDisableAutoMigrateBatching(t *testing.T) {
	t.Parallel()

	server, _, serverTeardown := setupMockedTestServer(t)
	dialector := NewWithSpannerConfig(postgres.Config{
		DSN: fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true;%s", server.Address, ""),
	}, SpannerConfig{
		DisableAutoMigrateBatching: true,
	})
	db, server, teardown := setupTestGormConnectionWithDialector(t, server, serverTeardown, dialector)
	defer teardown()

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

	err = db.Migrator().AutoMigrate(&singer{}, &album{}, &test{})
	if err != nil {
		t.Fatal(err)
	}
	requests := server.TestDatabaseAdmin.Reqs()
	// We should have 5 individual DDL requests, instead of one batch with 5 statements.
	if g, w := len(requests), 5; g != w {
		t.Fatalf("request count mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestMigratorError(t *testing.T) {
	t.Parallel()

	db, _, teardown := setupTestGormConnection(t)
	defer teardown()

	// Close the underlying database connection to force an error when getting a migrator.
	sqlDb, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	_ = sqlDb.Close()

	err = db.Migrator().AutoMigrate(&singer{}, &album{}, &test{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

type pgColumnTypesSpacedModel struct {
	ID uint `gorm:"primarykey"`
}

func (pgColumnTypesSpacedModel) TableName() string {
	return "singers WHERE 1=1; --"
}

const (
	pgColsMetaSQL = "SELECT c.column_name, c.is_nullable = 'YES', c.udt_name, c.character_maximum_length, c.numeric_precision, c.numeric_precision_radix, c.numeric_scale, c.datetime_precision, 8 * typlen, c.column_default, pd.description, c.identity_increment FROM information_schema.columns AS c LEFT JOIN pg_catalog.pg_type AS pgt ON c.udt_name = pgt.typname LEFT JOIN pg_catalog.pg_description as pd ON pd.objsubid = c.ordinal_position AND pd.objoid = (SELECT oid FROM pg_catalog.pg_class WHERE relname = c.table_name AND relnamespace = (SELECT oid FROM pg_catalog.pg_namespace WHERE nspname = c.table_schema)) where table_schema = $1 AND table_name = $2 and not column_name='spanner_gorm_generated_id'"
	pgUniqueSQL   = "SELECT constraint_name FROM information_schema.table_constraints tc JOIN information_schema.constraint_column_usage AS ccu USING (constraint_schema, constraint_catalog, table_name, constraint_name) JOIN information_schema.columns AS c ON c.table_schema = tc.constraint_schema AND tc.table_name = c.table_name AND ccu.column_name = c.column_name WHERE constraint_type IN ('PRIMARY KEY', 'UNIQUE') AND c.table_schema = $1 AND c.table_name = $2 AND constraint_type = $3 and not c.column_name='spanner_gorm_generated_id'"
	pgPKUniqueSQL = "SELECT c.column_name, constraint_name, constraint_type FROM information_schema.table_constraints tc JOIN information_schema.constraint_column_usage AS ccu USING (constraint_schema, constraint_catalog, table_name, constraint_name) JOIN information_schema.columns AS c ON c.table_schema = tc.constraint_schema AND tc.table_name = c.table_name AND ccu.column_name = c.column_name WHERE constraint_type IN ('PRIMARY KEY', 'UNIQUE') AND c.table_schema = $1 AND c.table_name = $2 and not c.column_name='spanner_gorm_generated_id'"
	pgDataTypeSQL = `select column_name, spanner_type as data_type
                                                 from information_schema.columns
                                                 where   table_schema=$1
                                                 and     table_name=$2
                                                 and not column_name='spanner_gorm_generated_id'`
)

func drainPGExecSQLs(server *testutil.MockedSpannerInMemTestServer) []string {
	var execSQLs []string
	for {
		select {
		case req := <-server.TestSpanner.ReceivedRequests():
			if execReq, ok := req.(*spannerpb.ExecuteSqlRequest); ok {
				execSQLs = append(execSQLs, execReq.GetSql())
			}
		default:
			return execSQLs
		}
	}
}

func pgEmptyResultSet(cols ...string) *testutil.StatementResult {
	fields := make([]*spannerpb.StructType_Field, 0, len(cols))
	for _, col := range cols {
		fields = append(fields, &spannerpb.StructType_Field{
			Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING},
			Name: col,
		})
	}
	return &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{Fields: fields},
			},
			Rows: []*structpb.ListValue{},
		},
	}
}

func registerPGColumnTypesEmptyMocks(t *testing.T, server *testutil.MockedSpannerInMemTestServer, selectSQL string) {
	t.Helper()
	for query, result := range map[string]*testutil.StatementResult{
		pgColsMetaSQL: pgEmptyResultSet("column_name", "is_nullable", "udt_name", "character_maximum_length", "numeric_precision", "numeric_precision_radix", "numeric_scale", "datetime_precision", "typlen", "column_default", "description", "identity_increment"),
		selectSQL:     pgEmptyResultSet("id"),
		pgUniqueSQL:   pgEmptyResultSet("constraint_name"),
		pgPKUniqueSQL: pgEmptyResultSet("column_name", "constraint_name", "constraint_type"),
		pgDataTypeSQL: pgEmptyResultSet("column_name", "data_type"),
	} {
		if err := server.TestSpanner.PutStatementResult(query, result); err != nil {
			t.Fatalf("PutStatementResult failed for %q: %v", query, err)
		}
	}
}

type pgColumnTypesDoubleQuoteModel struct {
	ID uint `gorm:"primarykey"`
}

func (pgColumnTypesDoubleQuoteModel) TableName() string {
	return `singers" WHERE 1=1; --`
}

func TestSpannerPostgresMigrator_ColumnTypes_TableQuoting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		value         interface{}
		wantSelectSQL string
	}{
		{
			name:          "simple table name",
			value:         "singers",
			wantSelectSQL: `SELECT * FROM "public"."singers" LIMIT $1`,
		},
		{
			name:          "table name with space",
			value:         "my singers",
			wantSelectSQL: `SELECT * FROM "public"."my singers" LIMIT $1`,
		},
		{
			name:          "SQL injection payload with space and WHERE",
			value:         "singers WHERE 1=1",
			wantSelectSQL: `SELECT * FROM "public"."singers WHERE 1=1" LIMIT $1`,
		},
		{
			name:          "backtick in table name",
			value:         "singers` WHERE 1=1",
			wantSelectSQL: "SELECT * FROM \"public\".\"singers` WHERE 1=1\" LIMIT $1",
		},
		{
			name:          "double quote injection payload",
			value:         `singers" WHERE 1=1`,
			wantSelectSQL: `SELECT * FROM "public"."singers"" WHERE 1=1" LIMIT $1`,
		},
		{
			name:          "leading double-quote injection payload",
			value:         `"" WHERE 1=1`,
			wantSelectSQL: `SELECT * FROM "public".""""" WHERE 1=1" LIMIT $1`,
		},
		{
			name:          "table name with multiple dots",
			value:         "a.b.c",
			wantSelectSQL: `SELECT * FROM "public"."a.b.c" LIMIT $1`,
		},
		{
			name:          "current table sentinel",
			value:         "~~~ct~~~",
			wantSelectSQL: `SELECT * FROM "public"."~~~ct~~~" LIMIT $1`,
		},
		{
			name:          "leading dot table name",
			value:         ".singers",
			wantSelectSQL: `SELECT * FROM "singers" LIMIT $1`,
		},
		{
			name:          "schema-qualified table name",
			value:         "myschema.singers",
			wantSelectSQL: `SELECT * FROM "myschema"."singers" LIMIT $1`,
		},
		{
			name:          "quote injection in schema",
			value:         `my"schema.singers`,
			wantSelectSQL: `SELECT * FROM "my""schema"."singers" LIMIT $1`,
		},
		{
			name:          "model with spaced injection TableName",
			value:         &pgColumnTypesSpacedModel{},
			wantSelectSQL: `SELECT * FROM "public"."singers WHERE 1=1; --" LIMIT $1`,
		},
		{
			name:          "model with double-quote injection TableName",
			value:         &pgColumnTypesDoubleQuoteModel{},
			wantSelectSQL: `SELECT * FROM "public"."singers"" WHERE 1=1; --" LIMIT $1`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, server, teardown := setupTestGormConnectionWithParams(t, "dialect=POSTGRESQL")
			defer teardown()
			drainPGExecSQLs(server)

			registerPGColumnTypesEmptyMocks(t, server, tc.wantSelectSQL)

			if _, err := db.Migrator().ColumnTypes(tc.value); err != nil {
				t.Fatalf("ColumnTypes(%v) failed: %v", tc.value, err)
			}

			execSQLs := drainPGExecSQLs(server)
			if g, w := len(execSQLs), 5; g != w {
				t.Fatalf("ExecuteSqlRequest count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := execSQLs[1], tc.wantSelectSQL; g != w {
				t.Errorf("GetRows SQL mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestSpannerPostgresMigrator_ColumnTypes_EmptyTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value interface{}
	}{
		{name: "empty string", value: ""},
		{name: "trailing dot with empty table", value: "myschema."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, server, teardown := setupTestGormConnectionWithParams(t, "dialect=POSTGRESQL")
			defer teardown()
			drainPGExecSQLs(server)

			if _, err := db.Migrator().ColumnTypes(tc.value); err == nil {
				t.Fatalf("expected error for %v, got nil", tc.value)
			}

			if g, w := len(drainPGExecSQLs(server)), 0; g != w {
				t.Errorf("ExecuteSqlRequest count mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestSpannerPostgresMigrator_ColumnTypes_IgnoresPriorSessionClauses(t *testing.T) {
	t.Parallel()

	const wantSelectSQL = `SELECT * FROM "public"."singers" LIMIT $1`

	tests := []struct {
		name    string
		buildDB func(*gorm.DB) *gorm.DB
	}{
		{
			name: "prior Table with vars, Where, and Select",
			buildDB: func(db *gorm.DB) *gorm.DB {
				return db.Table("singers WHERE ? = 1", 1).Where("active = ?", true).Select("id")
			},
		},
		{
			name: "prior Raw with vars",
			buildDB: func(db *gorm.DB) *gorm.DB {
				return db.Raw("SELECT ?", 99)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, server, teardown := setupTestGormConnectionWithParams(t, "dialect=POSTGRESQL")
			defer teardown()
			drainPGExecSQLs(server)

			registerPGColumnTypesEmptyMocks(t, server, wantSelectSQL)

			if _, err := tc.buildDB(db).Migrator().ColumnTypes(&singer{}); err != nil {
				t.Fatalf("ColumnTypes failed: %v", err)
			}

			execSQLs := drainPGExecSQLs(server)
			if g, w := len(execSQLs), 5; g != w {
				t.Fatalf("ExecuteSqlRequest count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := execSQLs[1], wantSelectSQL; g != w {
				t.Errorf("GetRows SQL mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestSpannerPostgresMigrator_ColumnTypes_DryRun(t *testing.T) {
	t.Parallel()

	const wantSelectSQL = `SELECT * FROM "public"."singers" LIMIT $1`

	db, server, teardown := setupTestGormConnectionWithParams(t, "dialect=POSTGRESQL")
	defer teardown()
	drainPGExecSQLs(server)

	registerPGColumnTypesEmptyMocks(t, server, wantSelectSQL)

	if _, err := db.Session(&gorm.Session{DryRun: true}).Migrator().ColumnTypes(&singer{}); err != nil {
		t.Fatalf("ColumnTypes under DryRun failed: %v", err)
	}

	execSQLs := drainPGExecSQLs(server)
	if g, w := len(execSQLs), 5; g != w {
		t.Fatalf("ExecuteSqlRequest count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := execSQLs[1], wantSelectSQL; g != w {
		t.Errorf("GetRows SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestSpannerPostgresMigrator_ColumnTypes_AssignsSQLColumnType(t *testing.T) {
	t.Parallel()

	const wantSelectSQL = `SELECT * FROM "public"."singers" LIMIT $1`

	db, server, teardown := setupTestGormConnectionWithParams(t, "dialect=POSTGRESQL")
	defer teardown()
	drainPGExecSQLs(server)

	registerPGColumnTypesEmptyMocks(t, server, wantSelectSQL)
	if err := server.TestSpanner.PutStatementResult(pgColsMetaSQL, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "column_name"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_BOOL}, Name: "is_nullable"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "udt_name"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "character_maximum_length"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "numeric_precision"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "numeric_precision_radix"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "numeric_scale"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "datetime_precision"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "typlen"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "column_default"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "description"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "identity_increment"},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "id"}},
					{Kind: &structpb.Value_BoolValue{BoolValue: false}},
					{Kind: &structpb.Value_StringValue{StringValue: "int8"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
			},
		},
	}); err != nil {
		t.Fatalf("PutStatementResult failed for colsMetaSQL: %v", err)
	}
	if err := server.TestSpanner.PutStatementResult(wantSelectSQL, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "id"},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "1"}},
				}},
			},
		},
	}); err != nil {
		t.Fatalf("PutStatementResult failed for selectSQL: %v", err)
	}

	cols, err := db.Migrator().ColumnTypes(&singer{})
	if err != nil {
		t.Fatalf("ColumnTypes failed: %v", err)
	}
	if g, w := len(cols), 1; g != w {
		t.Fatalf("column count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := cols[0].Name(), "id"; g != w {
		t.Errorf("column name mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if cols[0].ScanType() == nil {
		t.Error("expected SQLColumnType ScanType to be populated, got nil")
	}
}

func TestSpannerPostgresMigrator_ColumnTypes_Error(t *testing.T) {
	t.Parallel()

	const wantSelectSQL = `SELECT * FROM "public"."singers" LIMIT $1`

	tests := []struct {
		name          string
		value         interface{}
		cancelContext bool
		metaErr       error
		probeErr      error
		wantCode      codes.Code
		wantExecCount int
	}{
		{
			name:          "nil value",
			value:         nil,
			wantExecCount: 0,
		},
		{
			name:          "canceled context",
			value:         &singer{},
			cancelContext: true,
			wantExecCount: 0,
		},
		{
			name:          "metadata query fails",
			value:         &singer{},
			metaErr:       status.Error(codes.InvalidArgument, "metadata query failed"),
			wantCode:      codes.InvalidArgument,
			wantExecCount: 1,
		},
		{
			name:          "probe query returns RPC error",
			value:         &singer{},
			probeErr:      status.Error(codes.InvalidArgument, "probe query failed"),
			wantCode:      codes.InvalidArgument,
			wantExecCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, server, teardown := setupTestGormConnectionWithParams(t, "dialect=POSTGRESQL")
			defer teardown()
			drainPGExecSQLs(server)

			migratorDB := db
			if tc.cancelContext {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				migratorDB = db.WithContext(ctx)
			} else if tc.value != nil {
				registerPGColumnTypesEmptyMocks(t, server, wantSelectSQL)
			}
			if tc.metaErr != nil {
				if err := server.TestSpanner.PutStatementResult(pgColsMetaSQL, &testutil.StatementResult{
					Type: testutil.StatementResultError,
					Err:  tc.metaErr,
				}); err != nil {
					t.Fatalf("PutStatementResult failed: %v", err)
				}
			}
			if tc.probeErr != nil {
				if err := server.TestSpanner.PutStatementResult(wantSelectSQL, &testutil.StatementResult{
					Type: testutil.StatementResultError,
					Err:  tc.probeErr,
				}); err != nil {
					t.Fatalf("PutStatementResult failed: %v", err)
				}
			}

			_, err := migratorDB.Migrator().ColumnTypes(tc.value)
			if err == nil {
				t.Fatal("expected error from ColumnTypes, got nil")
			}
			if tc.wantCode != codes.OK {
				if g, w := spanner.ErrCode(err), tc.wantCode; g != w {
					t.Fatalf("ColumnTypes error code mismatch\nGot:  %v (%v)\nWant: %v", g, err, w)
				}
			}
			if g, w := len(drainPGExecSQLs(server)), tc.wantExecCount; g != w {
				t.Errorf("ExecuteSqlRequest count mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestSpannerPostgresMigrator_GetRows_InvalidTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		table interface{}
	}{
		{name: "empty table name", table: ""},
		{name: "non-string table", table: 123},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _, teardown := setupTestGormConnectionWithParams(t, "dialect=POSTGRESQL")
			defer teardown()

			pgMigrator, ok := db.Migrator().(spannerPostgresMigrator)
			if !ok {
				t.Fatalf("expected spannerPostgresMigrator, got %T", db.Migrator())
			}
			if _, err := pgMigrator.GetRows("public", tc.table); err == nil {
				t.Fatalf("expected error for table %v, got nil", tc.table)
			}
		})
	}
}

func TestSpannerPostgresMigrator_GetRows_SchemaVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		currentSchema       interface{}
		table               string
		withoutQuotingCheck bool
		wantSQL             string
	}{
		{
			name:          "nil schema",
			currentSchema: nil,
			table:         "singers",
			wantSQL:       `SELECT * FROM "singers" LIMIT $1`,
		},
		{
			name:          "empty string schema",
			currentSchema: "",
			table:         "singers",
			wantSQL:       `SELECT * FROM "singers" LIMIT $1`,
		},
		{
			name:          "clause.Expr schema ignored",
			currentSchema: clause.Expr{SQL: "CURRENT_SCHEMA()"},
			table:         "singers",
			wantSQL:       `SELECT * FROM "singers" LIMIT $1`,
		},
		{
			name:                "WithoutQuotingCheck still quotes identifiers",
			currentSchema:       "public",
			table:               "singers WHERE 1=1",
			withoutQuotingCheck: true,
			wantSQL:             `SELECT * FROM "public"."singers WHERE 1=1" LIMIT $1`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server, _, serverTeardown := setupMockedTestServer(t)
			dialector := New(postgres.Config{
				DSN:                 fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true;dialect=POSTGRESQL", server.Address),
				WithoutQuotingCheck: tc.withoutQuotingCheck,
			})
			db, server, teardown := setupTestGormConnectionWithDialector(t, server, serverTeardown, dialector)
			defer teardown()
			drainPGExecSQLs(server)

			if err := server.TestSpanner.PutStatementResult(tc.wantSQL, pgEmptyResultSet("id")); err != nil {
				t.Fatalf("PutStatementResult failed: %v", err)
			}

			pgMigrator, ok := db.Migrator().(spannerPostgresMigrator)
			if !ok {
				t.Fatalf("expected spannerPostgresMigrator, got %T", db.Migrator())
			}
			rows, err := pgMigrator.GetRows(tc.currentSchema, tc.table)
			if err != nil {
				t.Fatalf("GetRows failed: %v", err)
			}
			if _, err := rows.ColumnTypes(); err != nil {
				_ = rows.Close()
				t.Fatalf("rows.ColumnTypes failed: %v", err)
			}
			if err := rows.Close(); err != nil {
				t.Fatalf("rows.Close failed: %v", err)
			}

			execSQLs := drainPGExecSQLs(server)
			if g, w := len(execSQLs), 1; g != w {
				t.Fatalf("ExecuteSqlRequest count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := execSQLs[0], tc.wantSQL; g != w {
				t.Errorf("GetRows SQL mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func setupTestGormConnection(t *testing.T) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	return setupTestGormConnectionWithParams(t, "")
}

func setupTestGormConnectionWithParams(t *testing.T, params string) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	server, _, serverTeardown := setupMockedTestServer(t)
	return setupTestGormConnectionWithDialector(t, server, serverTeardown, New(postgres.Config{
		DSN: fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true;%s", server.Address, params),
	}))
}

func setupTestGormConnectionWithDialector(t *testing.T, s *testutil.MockedSpannerInMemTestServer, serverTeardown func(), dialector gorm.Dialector) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	db, err := gorm.Open(
		dialector,
		&gorm.Config{
			PrepareStmt: true,
			Logger:      logger.Default.LogMode(logger.Silent),
		})
	if err != nil {
		serverTeardown()
		t.Fatal(err)
	}

	return db, s, func() {
		_ = db
		serverTeardown()
	}
}

func setupMockedTestServer(t *testing.T) (server *testutil.MockedSpannerInMemTestServer, client *spanner.Client, teardown func()) {
	return setupMockedTestServerWithConfig(t, spanner.ClientConfig{})
}

func setupMockedTestServerWithConfig(t *testing.T, config spanner.ClientConfig) (server *testutil.MockedSpannerInMemTestServer, client *spanner.Client, teardown func()) {
	return setupMockedTestServerWithConfigAndClientOptions(t, config, []option.ClientOption{})
}

func setupMockedTestServerWithConfigAndClientOptions(t *testing.T, config spanner.ClientConfig, clientOptions []option.ClientOption) (server *testutil.MockedSpannerInMemTestServer, client *spanner.Client, teardown func()) {
	server, opts, serverTeardown := testutil.NewMockedSpannerInMemTestServer(t)
	opts = append(opts, clientOptions...)
	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	client, err := spanner.NewClientWithConfig(ctx, formattedDatabase, config, opts...)
	if err != nil {
		t.Fatal(err)
	}

	dialectQuery := "select option_value from information_schema.database_options where option_name='database_dialect'"
	if err := server.TestSpanner.PutStatementResult(dialectQuery, &testutil.StatementResult{
		Type:      testutil.StatementResultResultSet,
		ResultSet: testutil.CreateSelectDialectResultSet(databasepb.DatabaseDialect_POSTGRESQL),
	}); err != nil {
		t.Fatal(err)
	}

	query := "select count(1) from information_schema.database_options where schema_name='public' and option_name='default_sequence_kind'"
	if err := server.TestSpanner.PutStatementResult(query, &testutil.StatementResult{
		Type:      testutil.StatementResultResultSet,
		ResultSet: testutil.CreateSelect1ResultSet(),
	}); err != nil {
		t.Fatal(err)
	}

	return server, client, func() {
		client.Close()
		serverTeardown()
	}
}
