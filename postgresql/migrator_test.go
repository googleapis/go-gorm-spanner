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
	"testing"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
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
