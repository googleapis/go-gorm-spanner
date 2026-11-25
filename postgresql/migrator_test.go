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
	"strings"
	"testing"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/go-sql-spanner/testutil"
	"google.golang.org/api/option"
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
