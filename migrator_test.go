// Copyright 2023 Google LLC
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

package gorm

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/go-sql-spanner/testutil"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
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
	ID       uint `gorm:"primarykey" gorm_sequence_name:"overrided_sequence_name"`
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
	if g, w := len(request.GetStatements()), 8; g != w {
		t.Fatalf("statement count mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := request.GetStatements()[0],
		`CREATE SEQUENCE IF NOT EXISTS singers_seq OPTIONS (sequence_kind = "bit_reversed_positive")`; g != w {
		t.Fatalf("create singers sequence statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := request.GetStatements()[1],
		"CREATE TABLE `singers` ("+
			"`id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence singers_seq)),`created_at` TIMESTAMP,`updated_at` TIMESTAMP,`deleted_at` TIMESTAMP,"+
			"`first_name` STRING(MAX),`last_name` STRING(MAX),`full_name` STRING(MAX),`active` BOOL) "+
			"PRIMARY KEY (`id`)"; g != w {
		t.Fatalf("create singers statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := request.GetStatements()[2],
		"CREATE INDEX `idx_singers_deleted_at` ON `singers`(`deleted_at`)"; g != w {
		t.Fatalf("create idx_singers_deleted_at statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := request.GetStatements()[3],
		`CREATE SEQUENCE IF NOT EXISTS albums_seq OPTIONS (sequence_kind = "bit_reversed_positive")`; g != w {
		t.Fatalf("create albums sequence statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := request.GetStatements()[4],
		"CREATE TABLE `albums` (`id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence albums_seq)),`created_at` TIMESTAMP,`updated_at` TIMESTAMP,`deleted_at` TIMESTAMP,"+
			"`title` STRING(MAX),`rating` FLOAT32,`singer_id` INT64,"+
			"CONSTRAINT `fk_albums_singer` FOREIGN KEY (`singer_id`) REFERENCES `singers`(`id`)) "+
			"PRIMARY KEY (`id`)"; g != w {
		t.Fatalf("create albums statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := request.GetStatements()[5],
		"CREATE INDEX `idx_albums_deleted_at` ON `albums`(`deleted_at`)"; g != w {
		t.Fatalf("create idx_albums_deleted_at statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := request.GetStatements()[6],
		`CREATE SEQUENCE IF NOT EXISTS overrided_sequence_name OPTIONS (sequence_kind = "bit_reversed_positive")`; g != w {
		t.Fatalf("create albums sequence statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := request.GetStatements()[7],
		"CREATE TABLE `tests` (`id` INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence overrided_sequence_name)),"+
			"`test` STRING(MAX),`singer_id` INT64,"+
			"CONSTRAINT `fk_tests_singer` FOREIGN KEY (`singer_id`) REFERENCES `singers`(`id`)) "+
			"PRIMARY KEY (`id`)"; g != w {
		t.Fatalf("create albums statement text mismatch\n Got: %s\nWant: %s", g, w)
	}
}

func TestMigrateMultipleTimes(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()
	anyProto, err := anypb.New(&emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	server.TestDatabaseAdmin.SetResps([]proto.Message{
		&longrunningpb.Operation{
			Name:   "test-operation-1",
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyProto},
		},
		&longrunningpb.Operation{
			Name:   "test-operation-2",
			Done:   true,
			Result: &longrunningpb.Operation_Response{Response: anyProto},
		},
	})
	hasTableSql := "SELECT count(*) FROM information_schema.tables WHERE table_schema = @p1 AND table_name = @p2 AND table_type = @p3"
	hasColSql := "SELECT count(*) FROM INFORMATION_SCHEMA.columns WHERE table_schema = @p1 AND table_name = @p2 AND column_name = @p3"
	selectSingerRow := "SELECT * FROM `singers` LIMIT 1"
	getColDetailsSql := "SELECT COLUMN_NAME, COLUMN_DEFAULT, IS_NULLABLE = 'YES',\n\t\t\t\t\t   REGEXP_REPLACE(SPANNER_TYPE, '\\\\(.*\\\\)', '') AS DATA_TYPE,\n\t\t\t\t\t   SAFE_CAST(REPLACE(REPLACE(REGEXP_EXTRACT(SPANNER_TYPE, '\\\\(.*\\\\)'), '(', ''), ')', '') AS INT64) AS COLUMN_LENGTH,\n\t\t\t\t\t   (SELECT IF(I.INDEX_TYPE='PRIMARY_KEY', 'PRI', 'UNI')\n\t\t\t\t\t\tFROM INFORMATION_SCHEMA.INDEXES I\n\t\t\t\t\t\tINNER JOIN INFORMATION_SCHEMA.INDEX_COLUMNS IC USING (TABLE_CATALOG, TABLE_SCHEMA, TABLE_NAME, INDEX_NAME)\n\t\t\t\t\t\tWHERE IC.TABLE_CATALOG=C.TABLE_CATALOG AND IC.TABLE_SCHEMA=IC.TABLE_SCHEMA AND IC.TABLE_NAME=C.TABLE_NAME AND IC.COLUMN_NAME=C.COLUMN_NAME\n\t\t\t\t\t\t  AND I.IS_UNIQUE\n\t\t\t\t\t\tORDER BY I.INDEX_TYPE\n\t\t\t\t\t\tLIMIT 1\n\t\t\t\t\t   ) AS KEY,\n                    FROM INFORMATION_SCHEMA.COLUMNS C WHERE TABLE_SCHEMA = @p1 AND TABLE_NAME = @p2 ORDER BY ORDINAL_POSITION"
	hasIndexSql := "SELECT count(*) FROM information_schema.indexes WHERE table_schema = @p1 AND table_name = @p2 AND index_name = @p3"

	_ = putCountStatementResult(server, hasTableSql, 0)

	err = db.Migrator().AutoMigrate(&singer{}, &album{}, &test{})
	// Verify that the first migration worked and executed the expected number of requests.
	if err != nil {
		t.Fatal(err)
	}
	requests := server.TestDatabaseAdmin.Reqs()
	if g, w := len(requests), 1; g != w {
		t.Fatalf("request count mismatch\n Got: %v\nWant: %v", g, w)
	}
	request := requests[0].(*databasepb.UpdateDatabaseDdlRequest)
	if g, w := len(request.GetStatements()), 8; g != w {
		t.Fatalf("statement count mismatch\n Got: %v\nWant: %v", g, w)
	}

	// Then auto-migrate again with an unchanged data model.
	// This should lead to zero changes.
	_ = putCountStatementResult(server, hasTableSql, 1)
	_ = putCountStatementResult(server, hasColSql, 1)
	_ = putSelectSingerRowResult(server, selectSingerRow)
	_ = putSingerColDetailsResult(server, getColDetailsSql)
	_ = putCountStatementResult(server, hasIndexSql, 1)

	err = db.Migrator().AutoMigrate(&singer{})
	if err != nil {
		t.Fatal(err)
	}

	// The number of requests should still be 1, as we have made no changes to the `singer` table and model.
	requests = server.TestDatabaseAdmin.Reqs()
	if g, w := len(requests), 1; g != w {
		t.Fatalf("request count mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func putCountStatementResult(server *testutil.MockedSpannerInMemTestServer, sql string, count int) error {
	return server.TestSpanner.PutStatementResult(sql, &testutil.StatementResult{
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
				{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: strconv.Itoa(count)}}}},
			},
		},
	})
}

func putSingerColDetailsResult(server *testutil.MockedSpannerInMemTestServer, sql string) error {
	return server.TestSpanner.PutStatementResult(sql, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "COLUMN_NAME"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "COLUMN_DEFAULT"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_BOOL}, Name: "IS_NULLABLE"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "DATA_TYPE"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "COLUMN_LENGTH"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "KEY"},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "id"}},
					{Kind: &structpb.Value_StringValue{StringValue: "GET_NEXT_SEQUENCE_VALUE(Sequence singers_seq)"}},
					{Kind: &structpb.Value_BoolValue{BoolValue: true}},
					{Kind: &structpb.Value_StringValue{StringValue: "INT64"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "created_at"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_BoolValue{BoolValue: true}},
					{Kind: &structpb.Value_StringValue{StringValue: "TIMESTAMP"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "updated_at"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_BoolValue{BoolValue: true}},
					{Kind: &structpb.Value_StringValue{StringValue: "TIMESTAMP"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "deleted_at"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_BoolValue{BoolValue: true}},
					{Kind: &structpb.Value_StringValue{StringValue: "TIMESTAMP"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "first_name"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_BoolValue{BoolValue: true}},
					{Kind: &structpb.Value_StringValue{StringValue: "STRING"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "last_name"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_BoolValue{BoolValue: true}},
					{Kind: &structpb.Value_StringValue{StringValue: "STRING"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "full_name"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_BoolValue{BoolValue: true}},
					{Kind: &structpb.Value_StringValue{StringValue: "STRING"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: "active"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_BoolValue{BoolValue: true}},
					{Kind: &structpb.Value_StringValue{StringValue: "BOOL"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_NullValue{}},
				}},
			},
		},
	})
}

func putSelectSingerRowResult(server *testutil.MockedSpannerInMemTestServer, sql string) error {
	return server.TestSpanner.PutStatementResult(sql, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "id"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_TIMESTAMP}, Name: "created_at"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_TIMESTAMP}, Name: "updated_at"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_TIMESTAMP}, Name: "deleted_at"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "first_name"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "last_name"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "full_name"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_BOOL}, Name: "active"},
					},
				},
			},
			Rows: []*structpb.ListValue{},
		},
	})
}

func setupTestGormConnection(t *testing.T) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	return setupTestGormConnectionWithParams(t, "")
}

func setupTestGormConnectionWithParams(t *testing.T, params string) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	server, _, serverTeardown := setupMockedTestServer(t)
	db, err := gorm.Open(New(Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true;%s", server.Address, params),
	}), &gorm.Config{PrepareStmt: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		serverTeardown()
		t.Fatal(err)
	}

	return db, server, func() {
		// TODO: Close database?
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
	return server, client, func() {
		client.Close()
		serverTeardown()
	}
}
