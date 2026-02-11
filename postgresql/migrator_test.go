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
	"github.com/googleapis/go-sql-spanner/testutil"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
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
