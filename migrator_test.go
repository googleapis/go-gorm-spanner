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
	"testing"

	"github.com/golang/protobuf/proto"
	emptypb "github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/anypb"
	"gorm.io/gorm"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"

	"github.com/googleapis/go-sql-spanner/testutil"
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
			"`title` STRING(MAX),`singer_id` INT64,"+
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

func setupTestGormConnection(t *testing.T) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	return setupTestGormConnectionWithParams(t, "")
}

func setupTestGormConnectionWithParams(t *testing.T, params string) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	server, _, serverTeardown := setupMockedTestServer(t)
	db, err := gorm.Open(New(Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true;%s", server.Address, params),
	}), &gorm.Config{PrepareStmt: true})
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
