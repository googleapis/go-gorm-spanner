// Copyright 2024 Google LLC.
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
	"context"
	"database/sql"
	"reflect"
	"strconv"
	"testing"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	structpb "github.com/golang/protobuf/ptypes/struct"
	spannerdriver "github.com/googleapis/go-sql-spanner"
	"github.com/googleapis/go-sql-spanner/testutil"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type entity struct {
	gorm.Model
	Name     string
	children []child
}

type child struct {
	gorm.Model
	Name     string
	Parent   entity
	ParentId int64
}

type singerWithCommitTimestamp struct {
	ID          int64
	FirstName   string
	LastName    string
	LastUpdated CommitTimestamp
	Rating      float32
}

func (singerWithCommitTimestamp) TableName() string {
	return "singers"
}

func TestCommitTimestamp(t *testing.T) {
	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	s := singerWithCommitTimestamp{
		FirstName: "First",
		LastName:  "Last",
	}
	_ = putSingerResult(server, "INSERT INTO `singers` (`first_name`,`last_name`,`last_updated`,`rating`) VALUES (@p1,@p2,PENDING_COMMIT_TIMESTAMP(),@p3) THEN RETURN `id`", s)
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("failed to create singer: %v", err)
	}
	if s.LastUpdated.Timestamp.Valid {
		t.Fatalf("unexpected commit timestamp returned from insert")
	}
}

func TestFloat32(t *testing.T) {
	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	s := singerWithCommitTimestamp{
		FirstName: "First",
		LastName:  "Last",
		Rating:    float32(3.14),
	}
	_ = putSingerResult(server, "INSERT INTO `singers` (`first_name`,`last_name`,`last_updated`,`rating`) VALUES (@p1,@p2,PENDING_COMMIT_TIMESTAMP(),@p3) THEN RETURN `id`", s)
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("failed to create singer: %v", err)
	}
	req := getLastSqlRequest(server)
	if g, w := len(req.Params.Fields), 3; g != w {
		t.Errorf("param value mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := len(req.Params.Fields), 3; g != w {
		t.Errorf("param value mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := req.Params.Fields["p3"].GetNumberValue(), float64(float32(3.14)); g != w {
		t.Errorf("rating value mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func putSingerResult(server *testutil.MockedSpannerInMemTestServer, sql string, s singerWithCommitTimestamp) error {
	return server.TestSpanner.PutStatementResult(sql, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "ID"},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: strconv.Itoa(int(s.ID))}},
				}},
			},
		},
	})
}

func TestInsertOrUpdate(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	insertSql := "INSERT INTO `entities` (`created_at`,`updated_at`,`deleted_at`,`name`) VALUES (@p1,@p2,@p3,@p4) THEN RETURN `id`"
	insertOrUpdateSql := "INSERT OR UPDATE INTO `entities` (`created_at`,`updated_at`,`deleted_at`,`name`,`id`) VALUES (@p1,@p2,@p3,@p4,@p5) THEN RETURN `id`"
	insertOrIgnoreSql := "INSERT OR IGNORE INTO `entities` (`created_at`,`updated_at`,`deleted_at`,`name`,`id`) VALUES (@p1,@p2,@p3,@p4,@p5) THEN RETURN `id`"
	id := 1
	_ = server.TestSpanner.PutStatementResult(insertSql, createEntityResult(id, "foo"))
	_ = server.TestSpanner.PutStatementResult(insertOrUpdateSql, createEntityResult(id, "bar"))
	_ = server.TestSpanner.PutStatementResult(insertOrIgnoreSql, createEntityResult(id, "bar"))

	v := entity{Name: "foo"}
	if err := db.Create(&v).Error; err != nil {
		t.Fatalf("failed to create 'foo': %v", err)
	}
	if g, w := v.ID, uint(id); g != w {
		t.Fatalf("ID mismatch\n Got: %v\nWant: %v", g, w)
	}
	// Ensure that we got a normal insert.
	if g, w := getLastSql(server), insertSql; g != w {
		t.Fatalf("expected insert statement mismatch\n Got: %v\nWant: %v", g, w)
	}

	// Now try to insert-or-update the same record.
	v.Name = "bar"
	if err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&v).Error; err != nil {
		t.Fatalf("insert-or-update failed: %v", err)
	}
	// Ensure that we got an insert-or-update.
	if g, w := getLastSql(server), insertOrUpdateSql; g != w {
		t.Fatalf("expected insert-or-update statement mismatch\n Got: %v\nWant: %v", g, w)
	}
	// Verify that the updated name was returned.
	if g, w := v.Name, "bar"; g != w {
		t.Fatalf("returned name mismatch\n Got: %v\nWant: %v", g, w)
	}

	// Now try to insert-or-ignore the same record.
	v.Name = "baz"
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&v).Error; err != nil {
		t.Fatalf("insert-or-ignore failed: %v", err)
	}
	// Ensure that we got an insert-or-ignore.
	if g, w := getLastSql(server), insertOrIgnoreSql; g != w {
		t.Fatalf("expected insert-or-ignore statement mismatch\n Got: %v\nWant: %v", g, w)
	}
	// Verify that the original name was returned.
	if g, w := v.Name, "bar"; g != w {
		t.Fatalf("returned name mismatch\n Got: %v\nWant: %v", g, w)
	}

	// Verify that we can insert a new value.
	v2Id := 2
	_ = server.TestSpanner.PutStatementResult(insertSql, createEntityResult(v2Id, "v2"))
	v2 := entity{Name: "v2"}
	if err := db.Create(&v2).Error; err != nil {
		t.Fatalf("failed to create 'v2': %v", err)
	}
	if g, w := v2.ID, uint(v2Id); g != w {
		t.Fatalf("ID mismatch\n Got: %v\nWant: %v", g, w)
	}
	// Ensure that we got a normal insert.
	if g, w := getLastSql(server), insertSql; g != w {
		t.Fatalf("expected insert statement mismatch\n Got: %v\nWant: %v", g, w)
	}

	// Verify that we get an error if we try to do something else than UpdateAll or DoNothing.
	for i, c := range []clause.OnConflict{
		{},
		{OnConstraint: "constr", UpdateAll: true},
		{Where: clause.Where{Exprs: []clause.Expression{}}, UpdateAll: true},
		{TargetWhere: clause.Where{Exprs: []clause.Expression{}}, UpdateAll: true},
	} {
		if err := db.Clauses(c).Create(v).Error; err == nil {
			t.Fatalf("%d: missing expected error for OnConstraint", i)
		}
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true, OnConstraint: "foo"}).Create(v).Error; err == nil {
		t.Fatalf("missing expected error for OnConstraint")
	}
}

func TestAutoSaveAssociations(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	insertChild := "INSERT INTO `children` (`created_at`,`updated_at`,`deleted_at`,`name`,`parent_id`) VALUES (@p1,@p2,@p3,@p4,@p5) THEN RETURN `id`"
	insertParent := "INSERT OR IGNORE INTO `entities` (`created_at`,`updated_at`,`deleted_at`,`name`) VALUES (@p1,@p2,@p3,@p4) THEN RETURN `id`"
	childId := 1
	parentId := 2
	_ = server.TestSpanner.PutStatementResult(insertChild, createEntityResult(childId, "c1"))
	_ = server.TestSpanner.PutStatementResult(insertParent, createEntityResult(parentId, "p1"))

	c := child{Name: "c1", Parent: entity{Name: "p1"}}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("failed to create 'c1': %v", err)
	}
	if g, w := c.ID, uint(childId); g != w {
		t.Fatalf("child ID mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := c.Parent.ID, uint(parentId); g != w {
		t.Fatalf("parent ID mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestRunTransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	s := singerWithCommitTimestamp{
		FirstName: "First",
		LastName:  "Last",
	}
	insertSql := "INSERT INTO `singers` (`first_name`,`last_name`,`last_updated`,`rating`) VALUES (@p1,@p2,PENDING_COMMIT_TIMESTAMP(),@p3) THEN RETURN `id`"
	_ = putSingerResult(server, insertSql, s)
	if err := RunTransaction(ctx, db, func(tx *gorm.DB) error {
		if err := tx.Create(&s).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Verify that the insert was only executed once.
	reqs := drainRequestsFromServer(server.TestSpanner)
	execReqs := requestsOfType(reqs, reflect.TypeOf(&spannerpb.ExecuteSqlRequest{}))
	insertReqs := filter(execReqs, insertSql)
	if g, w := len(insertReqs), 1; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}

	// Run the same transaction again, but now we simulate that Spanner aborted the transaction.
	server.TestSpanner.PutExecutionTime(testutil.MethodCommitTransaction, testutil.SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	attempts := 0
	if err := RunTransaction(ctx, db, func(tx *gorm.DB) error {
		attempts++
		if err := tx.Create(&s).Error; err != nil {
			return err
		}
		return nil
	}, &sql.TxOptions{}); err != nil {
		t.Fatal(err)
	}
	// Now verify that the insert was executed twice and that the function was called twice.
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempts mismatch\n Got: %v\nWant: %v", g, w)
	}
	reqs = drainRequestsFromServer(server.TestSpanner)
	execReqs = requestsOfType(reqs, reflect.TypeOf(&spannerpb.ExecuteSqlRequest{}))
	insertReqs = filter(execReqs, insertSql)
	if g, w := len(insertReqs), 2; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestRunTransactionWithNilAsOptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, _, teardown := setupTestGormConnection(t)
	defer teardown()

	if err := RunTransaction(ctx, db, func(tx *gorm.DB) error {
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCustomSpannerConfig(t *testing.T) {
	t.Parallel()

	configFunc := func(config *spanner.ClientConfig, opts *[]option.ClientOption) {
		config.QueryOptions = spanner.QueryOptions{Options: &spannerpb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1"}}
	}
	config := spannerdriver.ConnectorConfig{
		Project:      "p",
		Instance:     "i",
		Database:     "d",
		Params:       map[string]string{"useplaintext": "true"},
		Configurator: configFunc,
	}
	db, server, teardown := setupTestGormConnectionWithCustomConfig(t, config)
	defer teardown()

	s := singerWithCommitTimestamp{
		FirstName: "First",
		LastName:  "Last",
	}
	insertSql := "INSERT INTO `singers` (`first_name`,`last_name`,`last_updated`,`rating`) VALUES (@p1,@p2,PENDING_COMMIT_TIMESTAMP(),@p3) THEN RETURN `id`"
	_ = putSingerResult(server, insertSql, s)
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("failed to create singer: %v", err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	execReqs := requestsOfType(reqs, reflect.TypeOf(&spannerpb.ExecuteSqlRequest{}))
	insertReqs := filter(execReqs, insertSql)
	if g, w := len(insertReqs), 1; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := insertReqs[0].QueryOptions.OptimizerVersion, "1"; g != w {
		t.Fatalf("optimizer version mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func filter(requests []interface{}, sql string) (ret []*spannerpb.ExecuteSqlRequest) {
	for _, i := range requests {
		if req, ok := i.(*spannerpb.ExecuteSqlRequest); ok {
			if req.Sql == sql {
				ret = append(ret, req)
			}
		}
	}
	return ret
}

func getLastSql(server *testutil.MockedSpannerInMemTestServer) string {
	return getLastSqlRequest(server).Sql
}

func getLastSqlRequest(server *testutil.MockedSpannerInMemTestServer) *spannerpb.ExecuteSqlRequest {
	reqs := drainRequestsFromServer(server.TestSpanner)
	execReqs := requestsOfType(reqs, reflect.TypeOf(&spannerpb.ExecuteSqlRequest{}))
	if len(execReqs) > 0 {
		return execReqs[len(execReqs)-1].(*spannerpb.ExecuteSqlRequest)
	}
	return &spannerpb.ExecuteSqlRequest{}
}

func requestsOfType(requests []interface{}, t reflect.Type) []interface{} {
	res := make([]interface{}, 0)
	for _, req := range requests {
		if reflect.TypeOf(req) == t {
			res = append(res, req)
		}
	}
	return res
}

func drainRequestsFromServer(server testutil.InMemSpannerServer) []interface{} {
	var reqs []interface{}
loop:
	for {
		select {
		case req := <-server.ReceivedRequests():
			reqs = append(reqs, req)
		default:
			break loop
		}
	}
	return reqs
}

func createEntityResult(id int, name string) *testutil.StatementResult {
	return &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}, Name: "ID"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_TIMESTAMP}, Name: "created_at"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_TIMESTAMP}, Name: "updated_at"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_TIMESTAMP}, Name: "deleted_at"},
						{Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING}, Name: "name"},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{
					{Kind: &structpb.Value_StringValue{StringValue: strconv.Itoa(id)}},
					{Kind: &structpb.Value_StringValue{StringValue: "2000-01-01T00:00:00Z"}},
					{Kind: &structpb.Value_StringValue{StringValue: "2000-01-01T00:00:00Z"}},
					{Kind: &structpb.Value_NullValue{}},
					{Kind: &structpb.Value_StringValue{StringValue: name}},
				}},
			},
			Stats: &spannerpb.ResultSetStats{RowCount: &spannerpb.ResultSetStats_RowCountExact{RowCountExact: int64(1)}},
		},
	}
}
