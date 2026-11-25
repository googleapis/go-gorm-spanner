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
	"strings"
	"testing"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	spannergorm "github.com/googleapis/go-gorm-spanner"
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

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	sqlDB.SetMaxOpenConns(1)

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
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Errorf("InUse connection count mismatch after AutoMigrate\nGot:  %v\nWant: %v", g, w)
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

func setupSingleConnMigratorTest(t *testing.T) (*gorm.DB, *sql.DB, *testutil.MockedSpannerInMemTestServer, func()) {
	t.Helper()
	db, server, teardown := setupTestGormConnection(t)
	sqlDB, err := db.DB()
	if err != nil {
		teardown()
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	// Limit the pool to 1 open connection so any unclosed *sql.Conn immediately
	// leaves InUse == 1 and causes subsequent queries to block.
	sqlDB.SetMaxOpenConns(1)
	return db, sqlDB, server, func() {
		_ = sqlDB.Close()
		teardown()
	}
}

func putPGCountStatementResult(t *testing.T, server *testutil.MockedSpannerInMemTestServer, sqlStr string, count string) {
	t.Helper()
	if err := server.TestSpanner.PutStatementResult(sqlStr, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "count"},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: count}}}},
			},
		},
	}); err != nil {
		t.Fatalf("failed to register statement result: %v", err)
	}
}

func TestMigrator_NoConnectionLeak_NonBatchCalls(t *testing.T) {
	t.Parallel()

	db, sqlDB, server, teardown := setupSingleConnMigratorTest(t)
	defer teardown()

	putPGCountStatementResult(t, server, "SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2 AND table_type = $3", "0")

	if db.Migrator().HasTable(&singer{}) {
		t.Fatalf("expected HasTable to return false")
	}
	// Verify the single pooled connection was returned immediately (not pinned by Dialector.Migrator).
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Fatalf("InUse connection count mismatch after HasTable\nGot:  %v\nWant: %v", g, w)
	}

	// Execute a second query on the MaxOpenConns=1 pool to prove the pool is not exhausted.
	var one int64
	if err := db.Raw("SELECT 1").Scan(&one).Error; err != nil {
		t.Fatalf("expected subsequent query to succeed, got error: %v", err)
	}
	if g, w := one, int64(1); g != w {
		t.Errorf("SELECT 1 result mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestMigrator_NoConnectionLeak_AutoMigrate(t *testing.T) {
	t.Parallel()

	db, sqlDB, server, teardown := setupSingleConnMigratorTest(t)
	defer teardown()

	putPGCountStatementResult(t, server, "SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2 AND table_type = $3", "0")

	anyProto, err := anypb.New(&emptypb.Empty{})
	if err != nil {
		t.Fatalf("failed to create empty proto: %v", err)
	}
	server.TestDatabaseAdmin.SetResps([]proto.Message{
		&longrunningpb.Operation{
			Name:   "test-operation-1",
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyProto},
		},
	})

	if err := db.AutoMigrate(&singer{}); err != nil {
		t.Fatalf("first AutoMigrate failed: %v", err)
	}
	// Verify the dedicated *sql.Conn checked out for DDL batching was closed on completion.
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Fatalf("InUse connection count mismatch after first AutoMigrate\nGot:  %v\nWant: %v", g, w)
	}

	// Run a second AutoMigrate on the MaxOpenConns=1 pool to prove the pool is not exhausted.
	server.TestDatabaseAdmin.SetResps([]proto.Message{
		&longrunningpb.Operation{
			Name:   "test-operation-2",
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyProto},
		},
	})
	if err := db.Migrator().AutoMigrate(&album{}); err != nil {
		t.Fatalf("second AutoMigrate failed: %v", err)
	}
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Errorf("InUse connection count mismatch after second AutoMigrate\nGot:  %v\nWant: %v", g, w)
	}

	var one int64
	if err := db.Raw("SELECT 1").Scan(&one).Error; err != nil {
		t.Fatalf("expected subsequent query to succeed, got error: %v", err)
	}
}

func TestMigrator_NoConnectionLeak_AutoMigrate_DefaultSequenceKind(t *testing.T) {
	t.Parallel()

	db, sqlDB, server, teardown := setupSingleConnMigratorTest(t)
	defer teardown()

	// Override default_sequence_kind check to return 0 so AutoMigrate emits ALTER DATABASE.
	putPGCountStatementResult(t, server, "select count(1) from information_schema.database_options where schema_name='public' and option_name='default_sequence_kind'", "0")
	putPGCountStatementResult(t, server, "SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2 AND table_type = $3", "0")

	anyProto, err := anypb.New(&emptypb.Empty{})
	if err != nil {
		t.Fatalf("failed to create empty proto: %v", err)
	}
	server.TestDatabaseAdmin.SetResps([]proto.Message{
		&longrunningpb.Operation{
			Name:   "test-operation",
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyProto},
		},
	})

	if err := db.AutoMigrate(&singer{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Fatalf("InUse connection count mismatch after AutoMigrate\nGot:  %v\nWant: %v", g, w)
	}
	requests := server.TestDatabaseAdmin.Reqs()
	if g, w := len(requests), 1; g != w {
		t.Fatalf("DDL request count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	request := requests[0].(*databasepb.UpdateDatabaseDdlRequest)
	// Expect ALTER DATABASE + CREATE TABLE + CREATE INDEX in one batch.
	if g, w := len(request.GetStatements()), 3; g != w {
		t.Fatalf("DDL statement count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestMigrator_NoConnectionLeak_AutoMigrateError(t *testing.T) {
	t.Parallel()

	db, sqlDB, server, teardown := setupSingleConnMigratorTest(t)
	defer teardown()

	putPGCountStatementResult(t, server, "SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2 AND table_type = $3", "0")
	server.TestDatabaseAdmin.SetErr(status.Error(codes.InvalidArgument, "ddl batch failed"))

	if err := db.AutoMigrate(&singer{}); err == nil {
		t.Fatalf("expected AutoMigrate error, got nil")
	}
	// Verify the batch connection is still released when AutoMigrate fails.
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Fatalf("InUse connection count mismatch after failed AutoMigrate\nGot:  %v\nWant: %v", g, w)
	}

	// Subsequent query on the MaxOpenConns=1 pool proves no connection leaked on error.
	var one int64
	if err := db.Raw("SELECT 1").Scan(&one).Error; err != nil {
		t.Fatalf("expected subsequent query to succeed after failed AutoMigrate, got error: %v", err)
	}
}

func TestMigrator_NoConnectionLeak_AutoMigrateDryRun(t *testing.T) {
	t.Parallel()

	db, sqlDB, server, teardown := setupSingleConnMigratorTest(t)
	defer teardown()

	putPGCountStatementResult(t, server, "SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2 AND table_type = $3", "0")

	migrator, ok := db.Migrator().(spannergorm.SpannerMigrator)
	if !ok {
		t.Fatalf("expected SpannerMigrator")
	}
	statements, err := migrator.AutoMigrateDryRun(&singer{})
	if err != nil {
		t.Fatalf("AutoMigrateDryRun failed: %v", err)
	}
	if g, w := len(statements), 2; g != w {
		t.Fatalf("dry-run statement count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	// Verify the dedicated *sql.Conn checked out for dry-run batching was released after AbortBatch.
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Fatalf("InUse connection count mismatch after AutoMigrateDryRun\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := len(server.TestDatabaseAdmin.Reqs()), 0; g != w {
		t.Errorf("expected 0 admin DDL requests during dry-run\nGot:  %v\nWant: %v", g, w)
	}

	// Execute a query on the MaxOpenConns=1 pool to prove the pool is not exhausted.
	var one int64
	if err := db.Raw("SELECT 1").Scan(&one).Error; err != nil {
		t.Fatalf("expected subsequent query to succeed after AutoMigrateDryRun, got error: %v", err)
	}
}

func TestMigrator_NoConnectionLeak_ManualBatch(t *testing.T) {
	t.Parallel()

	db, sqlDB, server, teardown := setupSingleConnMigratorTest(t)
	defer teardown()

	anyProto, err := anypb.New(&emptypb.Empty{})
	if err != nil {
		t.Fatalf("failed to create empty proto: %v", err)
	}
	server.TestDatabaseAdmin.SetResps([]proto.Message{
		&longrunningpb.Operation{
			Name:   "test-operation",
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyProto},
		},
	})

	if err := db.Connection(func(tx *gorm.DB) error {
		migrator, ok := tx.Migrator().(spannergorm.SpannerMigrator)
		if !ok {
			return fmt.Errorf("expected SpannerMigrator")
		}
		if err := migrator.StartBatchDDL(); err != nil {
			return err
		}
		if err := migrator.CreateTable(&singer{}); err != nil {
			_ = migrator.AbortBatch()
			return err
		}
		return migrator.RunBatch()
	}); err != nil {
		t.Fatalf("manual RunBatch failed: %v", err)
	}
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Fatalf("InUse connection count mismatch after RunBatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := len(server.TestDatabaseAdmin.Reqs()), 1; g != w {
		t.Fatalf("DDL request count mismatch after RunBatch\nGot:  %v\nWant: %v", g, w)
	}

	if err := db.Connection(func(tx *gorm.DB) error {
		migrator, ok := tx.Migrator().(spannergorm.SpannerMigrator)
		if !ok {
			return fmt.Errorf("expected SpannerMigrator")
		}
		if err := migrator.StartBatchDDL(); err != nil {
			return err
		}
		if err := migrator.CreateTable(&album{}); err != nil {
			_ = migrator.AbortBatch()
			return err
		}
		return migrator.AbortBatch()
	}); err != nil {
		t.Fatalf("manual AbortBatch failed: %v", err)
	}
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Errorf("InUse connection count mismatch after AbortBatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := len(server.TestDatabaseAdmin.Reqs()), 1; g != w {
		t.Errorf("DDL request count mismatch after AbortBatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestMigrator_StartBatchDDLWithoutDedicatedConnError(t *testing.T) {
	t.Parallel()

	db, sqlDB, _, teardown := setupSingleConnMigratorTest(t)
	defer teardown()

	migrator, ok := db.Migrator().(spannergorm.SpannerMigrator)
	if !ok {
		t.Fatalf("expected SpannerMigrator")
	}
	if err := migrator.StartBatchDDL(); err == nil {
		t.Fatal("expected StartBatchDDL without dedicated connection to fail, got nil")
	}
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Errorf("InUse connection count mismatch\nGot:  %v\nWant: %v", g, w)
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

type customSchemaParent struct {
	ID   uint   `gorm:"primarykey"`
	Name string `gorm:"index:idx_shared_name"`
}

func (customSchemaParent) TableName() string {
	return "s1.parents"
}

type customSchemaChild struct {
	ID       uint `gorm:"primarykey"`
	ParentID uint
	Age      int                 `gorm:"check:chk_age,age > 0"`
	Parent   *customSchemaParent `gorm:"foreignKey:ParentID"`
}

func (customSchemaChild) TableName() string {
	return "s2.children"
}

type customSchemaParentWithHasMany struct {
	ID                     uint                  `gorm:"primarykey"`
	CustomChildren         []customSchemaChild   `gorm:"foreignKey:ParentID"`
	PublicChildren         []publicSchemaChild   `gorm:"foreignKey:ParentID"`
	PublicSameNameChildren []publicSameNameChild `gorm:"foreignKey:ParentID"`
}

func (customSchemaParentWithHasMany) TableName() string {
	return "s1.parents"
}

type publicSchemaChild struct {
	ID       uint `gorm:"primarykey"`
	ParentID uint
}

func (publicSchemaChild) TableName() string {
	return "children"
}

type publicSameNameChild struct {
	ID       uint `gorm:"primarykey"`
	ParentID uint
}

func (publicSameNameChild) TableName() string {
	return "parents"
}

type publicSchemaParent struct {
	ID   uint   `gorm:"primarykey"`
	Name string `gorm:"index:idx_shared_name"`
}

func (publicSchemaParent) TableName() string {
	return "parents"
}

func enqueueDdlSuccessResponses(t *testing.T, server *testutil.MockedSpannerInMemTestServer, count int) {
	t.Helper()
	anyProto, err := anypb.New(&emptypb.Empty{})
	if err != nil {
		t.Fatalf("failed to create empty proto: %v", err)
	}
	responses := make([]proto.Message, count)
	for i := range responses {
		responses[i] = &longrunningpb.Operation{
			Name:   fmt.Sprintf("test-operation-%d", i),
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyProto},
		}
	}
	server.TestDatabaseAdmin.SetResps(responses)
}

func putDropTableDependencyResults(
	t *testing.T,
	server *testutil.MockedSpannerInMemTestServer,
	fkRows [][]string,
	indexNames []string,
) {
	t.Helper()
	fkQuery := "select tc.table_schema, tc.table_name, ctu.constraint_name " +
		"from information_schema.constraint_table_usage ctu " +
		"inner join information_schema.table_constraints tc using (constraint_catalog, constraint_schema, constraint_name)" +
		"WHERE ctu.table_schema = $1 AND ctu.table_name = $2 AND tc.constraint_type in ('FOREIGN KEY')"
	fkListRows := make([]*structpb.ListValue, 0, len(fkRows))
	for _, row := range fkRows {
		fkListRows = append(fkListRows, &structpb.ListValue{
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: row[0]}},
				{Kind: &structpb.Value_StringValue{StringValue: row[1]}},
				{Kind: &structpb.Value_StringValue{StringValue: row[2]}},
			},
		})
	}
	if err := server.TestSpanner.PutStatementResult(fkQuery, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "table_schema"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "table_name"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "constraint_name"},
					},
				},
			},
			Rows: fkListRows,
		},
	}); err != nil {
		t.Fatalf("failed to register FK query result: %v", err)
	}

	idxQuery := "select index_name from information_schema.indexes WHERE table_schema = $1 AND table_name = $2 AND index_type in ('INDEX') AND spanner_is_managed='NO'"
	idxListRows := make([]*structpb.ListValue, 0, len(indexNames))
	for _, name := range indexNames {
		idxListRows = append(idxListRows, &structpb.ListValue{
			Values: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: name}},
			},
		})
	}
	if err := server.TestSpanner.PutStatementResult(idxQuery, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "index_name"},
					},
				},
			},
			Rows: idxListRows,
		},
	}); err != nil {
		t.Fatalf("failed to register index query result: %v", err)
	}
}

func collectDdlStatements(server *testutil.MockedSpannerInMemTestServer) []string {
	var statements []string
	for _, req := range server.TestDatabaseAdmin.Reqs() {
		ddlReq, ok := req.(*databasepb.UpdateDatabaseDdlRequest)
		if !ok {
			continue
		}
		statements = append(statements, ddlReq.GetStatements()...)
	}
	return statements
}

const (
	dropTableFKQuery = "select tc.table_schema, tc.table_name, ctu.constraint_name " +
		"from information_schema.constraint_table_usage ctu " +
		"inner join information_schema.table_constraints tc using (constraint_catalog, constraint_schema, constraint_name)" +
		"WHERE ctu.table_schema = $1 AND ctu.table_name = $2 AND tc.constraint_type in ('FOREIGN KEY')"
	dropTableIdxQuery = "select index_name from information_schema.indexes WHERE table_schema = $1 AND table_name = $2 AND index_type in ('INDEX') AND spanner_is_managed='NO'"
)

func setupPostgresTestGormConnection(t *testing.T) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	t.Helper()
	server, _, serverTeardown := setupMockedTestServer(t)
	server.SetupSelectDialectResult(databasepb.DatabaseDialect_POSTGRESQL)
	return setupTestGormConnectionWithDialector(t, server, serverTeardown, New(postgres.Config{
		DSN: fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true", server.Address),
	}))
}

func drainExecutedSQLRequests(server *testutil.MockedSpannerInMemTestServer) []*spannerpb.ExecuteSqlRequest {
	var all []*spannerpb.ExecuteSqlRequest
	for {
		select {
		case req := <-server.TestSpanner.ReceivedRequests():
			if execReq, ok := req.(*spannerpb.ExecuteSqlRequest); ok {
				all = append(all, execReq)
			}
		default:
			return all
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

func filterExecutedSQLRequests(requests []*spannerpb.ExecuteSqlRequest, sql string) []*spannerpb.ExecuteSqlRequest {
	var matched []*spannerpb.ExecuteSqlRequest
	for _, req := range requests {
		if req.Sql == sql {
			matched = append(matched, req)
		}
	}
	return matched
}

func TestDropTable_SchemaQualification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		target         interface{}
		wantSchema     string
		wantTable      string
		fkRows         [][]string
		indexNames     []string
		wantStatements []string
	}{
		{
			name:       "custom_schema_model_with_cross_schema_fk_and_index",
			target:     &customSchemaParent{},
			wantSchema: "s1",
			wantTable:  "parents",
			fkRows: [][]string{
				{"s2", "children", "fk_s2_children_parent"},
				{"public", "other_children", "fk_other_children_parent"},
			},
			indexNames: []string{"idx_shared_name"},
			wantStatements: []string{
				`ALTER TABLE "s2"."children" DROP CONSTRAINT "fk_s2_children_parent"`,
				`ALTER TABLE "public"."other_children" DROP CONSTRAINT "fk_other_children_parent"`,
				`DROP INDEX "s1"."idx_shared_name"`,
				`DROP TABLE IF EXISTS "s1"."parents"`,
			},
		},
		{
			name:       "custom_schema_string_table_name",
			target:     "s1.parents",
			wantSchema: "s1",
			wantTable:  "parents",
			fkRows: [][]string{
				{"s2", "children", "fk_s2_children_parent"},
			},
			indexNames: []string{"idx_shared_name"},
			wantStatements: []string{
				`ALTER TABLE "s2"."children" DROP CONSTRAINT "fk_s2_children_parent"`,
				`DROP INDEX "s1"."idx_shared_name"`,
				`DROP TABLE IF EXISTS "s1"."parents"`,
			},
		},
		{
			name:       "public_schema_model_with_cross_schema_fk_and_index",
			target:     &publicSchemaParent{},
			wantSchema: "public",
			wantTable:  "parents",
			fkRows: [][]string{
				{"s2", "cross_children", "fk_cross_children_parent"},
			},
			indexNames: []string{"idx_shared_name"},
			wantStatements: []string{
				`ALTER TABLE "s2"."cross_children" DROP CONSTRAINT "fk_cross_children_parent"`,
				`DROP INDEX "public"."idx_shared_name"`,
				`DROP TABLE IF EXISTS "parents"`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, server, teardown := setupPostgresTestGormConnection(t)
			defer teardown()

			putDropTableDependencyResults(t, server, tc.fkRows, tc.indexNames)
			enqueueDdlSuccessResponses(t, server, len(tc.wantStatements))

			if err := db.Migrator().DropTable(tc.target); err != nil {
				t.Fatalf("DropTable failed: %v", err)
			}

			if g, w := collectDdlStatements(server), tc.wantStatements; !cmp.Equal(g, w) {
				t.Errorf("DDL statements mismatch\nGot:  %v\nWant: %v", g, w)
			}

			allSQLRequests := drainExecutedSQLRequests(server)
			wantParams := []string{tc.wantSchema, tc.wantTable}
			for _, query := range []string{dropTableFKQuery, dropTableIdxQuery} {
				reqs := filterExecutedSQLRequests(allSQLRequests, query)
				if g, w := len(reqs), 1; g != w {
					t.Fatalf("query %q request count mismatch\nGot:  %v\nWant: %v", query, g, w)
				}
				gotParams := []string{
					reqs[0].GetParams().GetFields()["p1"].GetStringValue(),
					reqs[0].GetParams().GetFields()["p2"].GetStringValue(),
				}
				if g, w := gotParams, wantParams; !cmp.Equal(g, w) {
					t.Errorf("query %q params mismatch\nGot:  %v\nWant: %v", query, g, w)
				}
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

func TestDropConstraint_SchemaQualification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		target         interface{}
		constraint     string
		wantStatements []string
	}{
		{
			name:       "check_constraint_on_custom_schema_model",
			target:     &customSchemaChild{},
			constraint: "chk_age",
			wantStatements: []string{
				`ALTER TABLE "s2"."children" DROP CONSTRAINT "chk_age"`,
			},
		},
		{
			name:       "belongs_to_fk_on_custom_schema_model",
			target:     &customSchemaChild{},
			constraint: "Parent",
			wantStatements: []string{
				`ALTER TABLE "s2"."children" DROP CONSTRAINT "fk_s2_children_parent"`,
			},
		},
		{
			name:       "has_many_fk_to_custom_schema_child",
			target:     &customSchemaParentWithHasMany{},
			constraint: "CustomChildren",
			wantStatements: []string{
				`ALTER TABLE "s2"."children" DROP CONSTRAINT "fk_s1_parents_custom_children"`,
			},
		},
		{
			name:       "has_many_fk_to_public_schema_child_does_not_inherit_parent_schema",
			target:     &customSchemaParentWithHasMany{},
			constraint: "PublicChildren",
			wantStatements: []string{
				`ALTER TABLE "public"."children" DROP CONSTRAINT "fk_s1_parents_public_children"`,
			},
		},
		{
			name:       "has_many_fk_to_same_unqualified_table_name_in_public_does_not_inherit_parent_schema",
			target:     &customSchemaParentWithHasMany{},
			constraint: "PublicSameNameChildren",
			wantStatements: []string{
				`ALTER TABLE "public"."parents" DROP CONSTRAINT "fk_s1_parents_public_same_name_children"`,
			},
		},
		{
			name:       "unmatched_raw_constraint_name_on_custom_schema_model",
			target:     &customSchemaChild{},
			constraint: "raw_fk",
			wantStatements: []string{
				`ALTER TABLE "s2"."children" DROP CONSTRAINT "raw_fk"`,
			},
		},
		{
			name:       "string_table_with_schema",
			target:     "s2.children",
			constraint: "fk_custom",
			wantStatements: []string{
				`ALTER TABLE "s2"."children" DROP CONSTRAINT "fk_custom"`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, server, teardown := setupPostgresTestGormConnection(t)
			defer teardown()

			enqueueDdlSuccessResponses(t, server, len(tc.wantStatements))

			if err := db.Migrator().DropConstraint(tc.target, tc.constraint); err != nil {
				t.Fatalf("DropConstraint failed: %v", err)
			}

			if g, w := collectDdlStatements(server), tc.wantStatements; !cmp.Equal(g, w) {
				t.Errorf("DDL statements mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestHasConstraint_SchemaQualification(t *testing.T) {
	t.Parallel()

	const hasConstraintQuery = "SELECT count(*) FROM INFORMATION_SCHEMA.table_constraints WHERE table_schema = $1 AND table_name = $2 AND constraint_name = $3"

	tests := []struct {
		name           string
		target         interface{}
		constraint     string
		wantSchema     string
		wantTable      string
		wantConstraint string
	}{
		{
			name:           "check_constraint_on_custom_schema_model",
			target:         &customSchemaChild{},
			constraint:     "chk_age",
			wantSchema:     "s2",
			wantTable:      "children",
			wantConstraint: "chk_age",
		},
		{
			name:           "has_many_fk_to_public_schema_child_does_not_inherit_parent_schema",
			target:         &customSchemaParentWithHasMany{},
			constraint:     "PublicChildren",
			wantSchema:     "public",
			wantTable:      "children",
			wantConstraint: "fk_s1_parents_public_children",
		},
		{
			name:           "has_many_fk_to_same_unqualified_table_name_in_public_does_not_inherit_parent_schema",
			target:         &customSchemaParentWithHasMany{},
			constraint:     "PublicSameNameChildren",
			wantSchema:     "public",
			wantTable:      "parents",
			wantConstraint: "fk_s1_parents_public_same_name_children",
		},
		{
			name:           "unmatched_raw_constraint_name_on_custom_schema_model",
			target:         &customSchemaChild{},
			constraint:     "raw_fk",
			wantSchema:     "s2",
			wantTable:      "children",
			wantConstraint: "raw_fk",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, server, teardown := setupPostgresTestGormConnection(t)
			defer teardown()

			if err := server.TestSpanner.PutStatementResult(hasConstraintQuery, &testutil.StatementResult{
				Type:      testutil.StatementResultResultSet,
				ResultSet: testutil.CreateSelect1ResultSet(),
			}); err != nil {
				t.Fatalf("failed to register HasConstraint query result: %v", err)
			}

			if g, w := db.Migrator().HasConstraint(tc.target, tc.constraint), true; g != w {
				t.Fatalf("HasConstraint mismatch\nGot:  %v\nWant: %v", g, w)
			}

			reqs := filterExecutedSQLRequests(drainExecutedSQLRequests(server), hasConstraintQuery)
			if g, w := len(reqs), 1; g != w {
				t.Fatalf("HasConstraint request count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			gotParams := []string{
				reqs[0].GetParams().GetFields()["p1"].GetStringValue(),
				reqs[0].GetParams().GetFields()["p2"].GetStringValue(),
				reqs[0].GetParams().GetFields()["p3"].GetStringValue(),
			}
			wantParams := []string{tc.wantSchema, tc.wantTable, tc.wantConstraint}
			if g, w := gotParams, wantParams; !cmp.Equal(g, w) {
				t.Errorf("HasConstraint query params mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestGetIndexes_SchemaQualification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     interface{}
		wantSchema string
		wantTable  string
	}{
		{
			name:       "custom_schema_model",
			target:     &customSchemaParent{},
			wantSchema: "s1",
			wantTable:  "parents",
		},
		{
			name:       "public_schema_model",
			target:     &publicSchemaParent{},
			wantSchema: "public",
			wantTable:  "parents",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, server, teardown := setupPostgresTestGormConnection(t)
			defer teardown()

			getIndexesQuery := strings.Replace(indexSql, "?", "$1", 1)
			getIndexesQuery = strings.Replace(getIndexesQuery, "?", "$2", 1)

			if err := server.TestSpanner.PutStatementResult(getIndexesQuery, &testutil.StatementResult{
				Type: testutil.StatementResultResultSet,
				ResultSet: &spannerpb.ResultSet{
					Metadata: &spannerpb.ResultSetMetadata{
						RowType: &spannerpb.StructType{
							Fields: []*spannerpb.StructType_Field{
								{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "table_name"},
								{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "index_name"},
								{Type: &spannerpb.Type{Code: spannerpb.TypeCode_BOOL}, Name: "non_unique"},
								{Type: &spannerpb.Type{Code: spannerpb.TypeCode_BOOL}, Name: "primary"},
								{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "column_name"},
							},
						},
					},
					Rows: []*structpb.ListValue{
						{Values: []*structpb.Value{
							{Kind: &structpb.Value_StringValue{StringValue: tc.wantTable}},
							{Kind: &structpb.Value_StringValue{StringValue: "idx_shared_name"}},
							{Kind: &structpb.Value_BoolValue{BoolValue: false}},
							{Kind: &structpb.Value_BoolValue{BoolValue: false}},
							{Kind: &structpb.Value_StringValue{StringValue: "name"}},
						}},
					},
				},
			}); err != nil {
				t.Fatalf("failed to register GetIndexes query result: %v", err)
			}

			indexes, err := db.Migrator().GetIndexes(tc.target)
			if err != nil {
				t.Fatalf("GetIndexes failed: %v", err)
			}
			if g, w := len(indexes), 1; g != w {
				t.Fatalf("GetIndexes count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := indexes[0].Name(), "idx_shared_name"; g != w {
				t.Errorf("GetIndexes name mismatch\nGot:  %v\nWant: %v", g, w)
			}

			reqs := filterExecutedSQLRequests(drainExecutedSQLRequests(server), getIndexesQuery)
			if g, w := len(reqs), 1; g != w {
				t.Fatalf("GetIndexes request count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			gotParams := []string{
				reqs[0].GetParams().GetFields()["p1"].GetStringValue(),
				reqs[0].GetParams().GetFields()["p2"].GetStringValue(),
			}
			wantParams := []string{tc.wantSchema, tc.wantTable}
			if g, w := gotParams, wantParams; !cmp.Equal(g, w) {
				t.Errorf("GetIndexes query params mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestIsColumnGenerated_SchemaQualification(t *testing.T) {
	t.Parallel()

	const isGeneratedQuery = "SELECT count(*) FROM INFORMATION_SCHEMA.columns WHERE table_schema = $1 AND table_name = $2 AND column_name = $3 AND generation_expression IS NOT NULL"

	tests := []struct {
		name       string
		target     interface{}
		field      string
		wantSchema string
		wantTable  string
		wantColumn string
	}{
		{
			name:       "custom_schema_model",
			target:     &customSchemaParent{},
			field:      "Name",
			wantSchema: "s1",
			wantTable:  "parents",
			wantColumn: "name",
		},
		{
			name:       "string_table_with_schema",
			target:     "s1.parents",
			field:      "name",
			wantSchema: "s1",
			wantTable:  "parents",
			wantColumn: "name",
		},
		{
			name:       "public_schema_model",
			target:     &publicSchemaParent{},
			field:      "Name",
			wantSchema: "public",
			wantTable:  "parents",
			wantColumn: "name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, server, teardown := setupPostgresTestGormConnection(t)
			defer teardown()

			if err := server.TestSpanner.PutStatementResult(isGeneratedQuery, &testutil.StatementResult{
				Type:      testutil.StatementResultResultSet,
				ResultSet: testutil.CreateSelect1ResultSet(),
			}); err != nil {
				t.Fatalf("failed to register isColumnGenerated query result: %v", err)
			}

			pgMigrator := db.Migrator().(spannerPostgresMigrator)
			if g, w := pgMigrator.isColumnGenerated(tc.target, tc.field), true; g != w {
				t.Fatalf("isColumnGenerated mismatch\nGot:  %v\nWant: %v", g, w)
			}

			reqs := filterExecutedSQLRequests(drainExecutedSQLRequests(server), isGeneratedQuery)
			if g, w := len(reqs), 1; g != w {
				t.Fatalf("isColumnGenerated request count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			gotParams := []string{
				reqs[0].GetParams().GetFields()["p1"].GetStringValue(),
				reqs[0].GetParams().GetFields()["p2"].GetStringValue(),
				reqs[0].GetParams().GetFields()["p3"].GetStringValue(),
			}
			wantParams := []string{tc.wantSchema, tc.wantTable, tc.wantColumn}
			if g, w := gotParams, wantParams; !cmp.Equal(g, w) {
				t.Errorf("isColumnGenerated query params mismatch\nGot:  %v\nWant: %v", g, w)
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
