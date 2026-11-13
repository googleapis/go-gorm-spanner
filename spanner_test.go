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
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/spansql"
	structpb "github.com/golang/protobuf/ptypes/struct"
	spannerdriver "github.com/googleapis/go-sql-spanner"
	spannerparser "github.com/googleapis/go-sql-spanner/parser"
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

func TestSelectForUpdate(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	query := "SELECT * FROM `singers` WHERE `singers`.`id` = @p1 AND `singers`.`deleted_at` IS NULL FOR UPDATE"
	_ = putSelectSingerRowResult(server, query)
	var s singer
	if err := db.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Find(&s, 1).Error; err != nil {
		t.Fatalf("failed to load singer FOR UPDATE lock: %v", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(requests, reflect.TypeOf(&spannerpb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 2; g != w {
		t.Fatalf("num execute requests mismatch\n Got: %v\nWant: %v", g, w)
	}
	request := executeRequests[1].(*spannerpb.ExecuteSqlRequest)
	if g, w := request.Sql, query; g != w {
		t.Fatalf("query mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestLastStatement(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	s := singerWithCommitTimestamp{
		FirstName: "First",
		LastName:  "Last",
	}
	_ = putSingerResult(server, "INSERT INTO `singers` (`first_name`,`last_name`,`last_updated`,`rating`) VALUES (@p1,@p2,PENDING_COMMIT_TIMESTAMP(),@p3) THEN RETURN `id`", s)
	// Skip default transactions to enable the use of auto commit and the last_statement optimization.
	if err := db.Session(&gorm.Session{SkipDefaultTransaction: true}).Create(&s).Error; err != nil {
		t.Fatalf("failed to create singer: %v", err)
	}
	request := getLastSqlRequest(server)
	if !request.LastStatement {
		t.Fatalf("last statement mismatch\n Got: %v\nWant: %v", request.LastStatement, true)
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

func TestDialector_QuoteTo(t *testing.T) {
	dialector := Dialector{}
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: ""},
		{name: "asterisk", input: "*", expected: "*"},
		{name: "table asterisk", input: "table.*", expected: "`table`.*"},
		{name: "quoted table asterisk", input: "`table`.*", expected: "`table`.*"},
		{name: "simple identifier", input: "users", expected: "`users`"},
		{name: "already quoted identifier", input: "`users`", expected: "`users`"},
		{name: "dotted identifier", input: "users.name", expected: "`users`.`name`"},
		{name: "already quoted dotted identifier", input: "`users`.`name`", expected: "`users`.`name`"},
		{name: "partially quoted dotted first", input: "`users`.name", expected: "`users`.`name`"},
		{name: "partially quoted dotted second", input: "users.`name`", expected: "`users`.`name`"},
		{name: "multi-part dotted identifier", input: "project.dataset.table.column", expected: "`project`.`dataset`.`table`.`column`"},
		{name: "identifier with dot in name", input: "`users.name`", expected: "`users.name`"},
		{name: "identifier with internal backtick", input: "foo`bar", expected: "`foo\\`bar`"},
		{name: "identifier with multiple internal backticks", input: "foo``bar", expected: "`foo\\`\\`bar`"},
		{name: "identifier ending with backtick", input: "foo`", expected: "`foo\\``"},
		{name: "single backtick", input: "`", expected: "`\\``"},
		{name: "two backticks", input: "``", expected: "`\\`\\``"},
		{name: "identifier with internal backslash", input: `foo\bar`, expected: "`foo\\\\bar`"},
		{name: "identifier ending with backslash", input: `foo\`, expected: "`foo\\\\`"},
		{name: "identifier with backslash and backtick", input: "foo\\`bar", expected: "`foo\\\\\\`bar`"},
		{name: "identifier with two backslashes and backtick", input: "foo\\\\`bar", expected: "`foo\\\\\\\\\\`bar`"},
		{name: "identifier with three backslashes and backtick", input: "foo\\\\\\`bar", expected: "`foo\\\\\\\\\\\\\\`bar`"},
		{name: "SQL injection quote breakout", input: "users` WHERE 1=1; --", expected: "`users\\` WHERE 1=1; --`"},
		{name: "SQL injection backslash quote breakout", input: "users\\`; DROP TABLE users; --", expected: "`users\\\\\\`; DROP TABLE users; --`"},
		{name: "SQL injection trailing backslash", input: `users\`, expected: "`users\\\\`"},
		{name: "SQL injection self-quoted breakout attempt", input: "`users` OR 1=1; --`", expected: "`\\`users\\` OR 1=1; --\\``"},
		// Comments and statement delimiters
		{name: "identifier with double-dash comment", input: "foo--bar", expected: "`foo--bar`"},
		{name: "identifier with block comment", input: "foo/*comment*/bar", expected: "`foo/*comment*/bar`"},
		{name: "identifier with hash comment", input: "foo#bar", expected: "`foo#bar`"},
		{name: "identifier with semicolon and comment", input: "foo; DROP TABLE users; --", expected: "`foo; DROP TABLE users; --`"},
		// Line feeds, carriage returns, tabs, and control characters
		{name: "identifier with newline", input: "foo\nbar", expected: "`foo\\nbar`"},
		{name: "identifier with carriage return and newline", input: "foo\r\nbar", expected: "`foo\\r\\nbar`"},
		{name: "identifier with tab", input: "foo\tbar", expected: "`foo\\tbar`"},
		{name: "identifier with null byte", input: "foo\x00bar", expected: "`foo\\x00bar`"},
		{name: "identifier with bell", input: "foo\abar", expected: "`foo\\abar`"},
		{name: "identifier with backspace", input: "foo\bbar", expected: "`foo\\bbar`"},
		{name: "identifier with form feed", input: "foo\fbar", expected: "`foo\\fbar`"},
		{name: "identifier with vertical tab", input: "foo\vbar", expected: "`foo\\vbar`"},
		{name: "identifier with other ASCII control characters", input: "foo\x01\x1b\x7fbar", expected: "`foo\\x01\\x1b\\x7fbar`"},
		{name: "identifier with newline and comment injection", input: "foo\n-- comment\nSELECT *", expected: "`foo\\n-- comment\\nSELECT *`"},
		// Long identifiers (> 127 bytes)
		{name: "long identifier exceeding 127 bytes", input: strings.Repeat("a", 200), expected: "`" + strings.Repeat("a", 200) + "`"},
		// Spaces, hyphens, and quotes
		{name: "identifier with spaces", input: "order items", expected: "`order items`"},
		{name: "identifier with hyphen", input: "order-date", expected: "`order-date`"},
		{name: "identifier with single and double quotes", input: `foo'bar"baz`, expected: "`foo'bar\"baz`"},
		{name: "identifier with dollar and at sign", input: "$amount@currency", expected: "`$amount@currency`"},
		{name: "identifier with forward slash", input: "path/to/column", expected: "`path/to/column`"},
		// Reserved keywords
		{name: "reserved keyword SELECT", input: "SELECT", expected: "`SELECT`"},
		{name: "reserved keyword WHERE", input: "WHERE", expected: "`WHERE`"},
		{name: "reserved keyword ORDER", input: "ORDER", expected: "`ORDER`"},
		// Unicode and emojis
		{name: "unicode japanese identifier", input: "名前", expected: "`名前`"},
		{name: "unicode accented identifier", input: "üñîçødé", expected: "`üñîçødé`"},
		{name: "unicode emoji identifier", input: "orders_🚀", expected: "`orders_🚀`"},
		{name: "dotted unicode identifier", input: "スキーマ.テーブル", expected: "`スキーマ`.`テーブル`"},
		// Dots and path edge cases
		{name: "trailing dot", input: "users.", expected: "`users`."},
		{name: "leading dot", input: ".users", expected: ".`users`"},
		{name: "single dot", input: ".", expected: "."},
		{name: "double dot", input: "..", expected: ".."},
		{name: "empty middle component", input: "users..name", expected: "`users`..`name`"},
		{name: "triple dots", input: "users...name", expected: "`users`...`name`"},
		{name: "quoted with triple dots", input: "`users`...`name`", expected: "`users`...`name`"},
		{name: "multi-level dotted path", input: "catalog.schema.table.column", expected: "`catalog`.`schema`.`table`.`column`"},
		{name: "leading backtick before dot", input: "`.a", expected: "`\\``.`a`"},
		{name: "backtick then dot", input: "`.", expected: "`\\``."},
		{name: "unquoted ending with backtick before dot", input: "foo`.bar", expected: "`foo\\``.`bar`"},
		{name: "quoted ending with backtick before dot", input: "`foo`.bar", expected: "`foo`.`bar`"},
		{name: "quoted ending with backtick before quoted dot", input: "`foo`.`bar`", expected: "`foo`.`bar`"},
		{name: "backslash and backtick before dot", input: "foo\\`.bar", expected: "`foo\\\\\\``.`bar`"},
		// Already-quoted escaped identifiers (idempotency)
		{name: "already quoted with escaped backtick", input: "`foo\\`bar`", expected: "`foo\\`bar`"},
		{name: "already quoted with escaped backslash", input: "`foo\\\\bar`", expected: "`foo\\\\bar`"},
		{name: "already quoted with simple escapes", input: "`foo\\a\\b\\f\\n\\r\\t\\v\\?\\\"\\'bar`", expected: "`foo\\a\\b\\f\\n\\r\\t\\v\\?\\\"\\'bar`"},
		{name: "already quoted with escaped backtick and dot", input: "`foo\\`.bar`", expected: "`foo\\`.bar`"},
		{name: "already quoted ending with escaped backtick", input: "`foo\\``", expected: "`foo\\``"},
		{name: "already quoted with hex escape", input: "`foo\\x00bar`", expected: "`foo\\x00bar`"},
		{name: "already quoted with uppercase hex escape", input: "`foo\\Xabbar`", expected: "`foo\\Xabbar`"},
		{name: "already quoted with octal escape", input: "`foo\\000bar`", expected: "`foo\\000bar`"},
		{name: "already quoted with octal escape 377", input: "`foo\\377bar`", expected: "`foo\\377bar`"},
		{name: "already quoted with unicode4 escape", input: "`foo\\u0041bar`", expected: "`foo\\u0041bar`"},
		{name: "already quoted with unicode8 escape", input: "`foo\\U0001F680bar`", expected: "`foo\\U0001F680bar`"},
		{name: "already quoted ending with escaped backslash", input: "`foo\\\\`", expected: "`foo\\\\`"},
		// Invalid escapes or unclosed quotes inside backticks
		{name: "backticked ending with trailing backslash", input: "`users\\`", expected: "`\\`users\\\\\\``"},
		{name: "backticked with invalid single-digit zero escape", input: "`foo\\0bar`", expected: "`\\`foo\\\\0bar\\``"},
		{name: "backticked with unknown escape", input: "`foo\\zbar`", expected: "`\\`foo\\\\zbar\\``"},
		{name: "backticked with invalid hex escape", input: "`foo\\xZZbar`", expected: "`\\`foo\\\\xZZbar\\``"},
		{name: "backticked with short hex escape before backtick", input: "`foo\\x4`", expected: "`\\`foo\\\\x4\\``"},
		{name: "backticked with invalid unicode4 hex escape", input: "`foo\\uZZZZbar`", expected: "`\\`foo\\\\uZZZZbar\\``"},
		{name: "backticked with invalid unicode8 hex escape", input: "`foo\\UZZZZZZZZbar`", expected: "`\\`foo\\\\UZZZZZZZZbar\\``"},
		{name: "backticked with surrogate unicode4 escape", input: "`foo\\uD800bar`", expected: "`\\`foo\\\\uD800bar\\``"},
		{name: "backticked with surrogate unicode8 escape", input: "`foo\\U0000D800bar`", expected: "`\\`foo\\\\U0000D800bar\\``"},
		{name: "backticked with out of range unicode8 escape", input: "`foo\\U00110000bar`", expected: "`\\`foo\\\\U00110000bar\\``"},
		{name: "backticked with out of range octal escape", input: "`foo\\400bar`", expected: "`\\`foo\\\\400bar\\``"},
		{name: "backticked with non-octal second digit escape", input: "`foo\\088bar`", expected: "`\\`foo\\\\088bar\\``"},
		{name: "backticked with non-octal third digit escape", input: "`foo\\008bar`", expected: "`\\`foo\\\\008bar\\``"},
		{name: "backticked with truncated hex escape", input: "`foo\\x`", expected: "`\\`foo\\\\x\\``"},
		{name: "backticked with truncated unicode escape", input: "`foo\\u12`", expected: "`\\`foo\\\\u12\\``"},
		{name: "backticked with truncated unicode8 escape", input: "`foo\\U1234`", expected: "`\\`foo\\\\U1234\\``"},
		{name: "backticked with truncated octal escape", input: "`foo\\00`", expected: "`\\`foo\\\\00\\``"},
		{name: "backticked unclosed after hex escape", input: "`foo\\x41", expected: "`\\`foo\\\\x41`"},
		{name: "backticked unclosed after unicode escape", input: "`foo\\u0041", expected: "`\\`foo\\\\u0041`"},
		{name: "backticked unclosed after octal escape", input: "`foo\\000", expected: "`\\`foo\\\\000`"},
		{name: "backticked with raw newline", input: "`foo\nbar`", expected: "`\\`foo\\nbar\\``"},
		{name: "backticked with raw DEL character", input: "`foo\x7fbar`", expected: "`\\`foo\\x7fbar\\``"},
		// Unquoted control bytes and raw binary sequences
		{name: "standalone carriage return", input: "foo\rbar", expected: "`foo\\rbar`"},
		{name: "invalid UTF-8 bytes", input: "\xff\xfe", expected: "`\xff\xfe`"},
		// Wildcard permutations
		{name: "deep qualified wildcard", input: "db.schema.table.*", expected: "`db`.`schema`.`table`.*"},
		{name: "asterisk dot asterisk", input: "*.*", expected: "*.*"},
		{name: "leading dot asterisk", input: ".*", expected: ".*"},
		{name: "double dot asterisk", input: "..*", expected: "..*"},
		{name: "multi asterisk", input: "foo.*.*", expected: "`foo`.*.*"},
		{name: "quoted wildcard", input: "`foo.*`", expected: "`foo.*`"},
		{name: "backtick before dot asterisk", input: "foo`.*", expected: "`foo\\``.*"},
		// Backtick edge cases
		{name: "unpaired leading backtick", input: "`foo", expected: "`\\`foo`"},
		{name: "three backticks", input: "```", expected: "`\\`\\`\\``"},
		{name: "four backticks", input: "````", expected: "`\\`\\`\\`\\``"},
		{name: "middle backtick with spaces", input: "foo ` bar", expected: "`foo \\` bar`"},
		{name: "quoted identifier immediately followed by comment", input: "`users`--", expected: "`\\`users\\`--`"},
		{name: "quoted identifier immediately followed by block comment", input: "`users`/*comment*/", expected: "`\\`users\\`/*comment*/`"},
		{name: "quoted identifier immediately followed by semicolon", input: "`users`;DROP", expected: "`\\`users\\`;DROP`"},
		{name: "rejected dotted quote followed by semicolon", input: "`foo.bar`; DROP TABLE users; --", expected: "`\\`foo`.`bar\\`; DROP TABLE users; --`"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var builder strings.Builder
			dialector.QuoteTo(&builder, testCase.input)
			quoted := builder.String()
			if g, w := quoted, testCase.expected; g != w {
				t.Errorf("quote mismatch\nGot:  %v\nWant: %v", g, w)
			}

			var idempotentBuilder strings.Builder
			dialector.QuoteTo(&idempotentBuilder, quoted)
			if g, w := idempotentBuilder.String(), quoted; g != w {
				t.Errorf("QuoteTo is not idempotent for %q\nGot:  %v\nWant: %v", quoted, g, w)
			}
		})
	}
}

func TestDialector_QuoteTo_GoogleSQLParser(t *testing.T) {
	statementParser, err := spannerparser.NewStatementParser(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, 0)
	if err != nil {
		t.Fatalf("failed to create StatementParser: %v", err)
	}

	dialector := Dialector{}
	testCases := []struct {
		name  string
		input string
	}{
		{name: "simple identifier", input: "users"},
		{name: "already quoted identifier", input: "`users`"},
		{name: "already quoted escaped backtick", input: "`foo\\`bar`"},
		{name: "already quoted dot in name", input: "`users.name`"},
		{name: "dotted identifier", input: "users.name"},
		{name: "already quoted dotted identifier", input: "`users`.`name`"},
		{name: "hyphenated identifier", input: "order-date"},
		{name: "spaced identifier", input: "order items"},
		{name: "internal backtick", input: "foo`bar"},
		{name: "internal backslash", input: `foo\bar`},
		{name: "newline", input: "foo\nbar"},
		{name: "standalone carriage return", input: "foo\rbar"},
		{name: "crlf", input: "foo\r\nbar"},
		{name: "tab", input: "foo\tbar"},
		{name: "null byte", input: "foo\x00bar"},
		{name: "control characters", input: "foo\a\b\f\v\x01\x1b\x7fbar"},
		{name: "quotes in identifier", input: `foo'bar"baz`},
		{name: "comments in identifier", input: "foo--bar/*baz*/#qux"},
		{name: "semicolon in identifier", input: "foo; DROP TABLE users; --"},
		{name: "three backticks", input: "```"},
		{name: "unicode japanese", input: "名前"},
		{name: "unicode accents", input: "üñîçødé"},
		{name: "reserved keyword", input: "SELECT"},
		{name: "table wildcard", input: "table.*"},
		{name: "backtick before table wildcard", input: "foo`.*"},
		{name: "long identifier", input: strings.Repeat("a", 200)},
		{name: "SQL injection quote breakout", input: "users` WHERE 1=1; --"},
		{name: "SQL injection backslash quote breakout", input: "users\\`; DROP TABLE users; --"},
		{name: "SQL injection self-quoted breakout attempt", input: "`users` OR 1=1; --`"},
		{name: "SQL injection newline comment and placeholder", input: "foo\n-- comment\nSELECT * WHERE id = ?"},
		{name: "SQL injection parameter prefix", input: "@injected_param`"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var builder strings.Builder
			dialector.QuoteTo(&builder, testCase.input)
			quoted := builder.String()

			sqlStatement := fmt.Sprintf("SELECT %s FROM test_table", quoted)
			convertedSQL, params, _, err := statementParser.ParseParameters(sqlStatement)
			if err != nil {
				t.Errorf("StatementParser rejected quoted identifier: %s (err: %v)", sqlStatement, err)
			}
			if g, w := len(params), 0; g != w {
				t.Errorf("unexpected parameter count for quoted identifier %q\nGot:  %v\nWant: %v", sqlStatement, g, w)
			}
			if g, w := convertedSQL, sqlStatement; g != w {
				t.Errorf("statement mutated by parser for quoted identifier\nGot:  %v\nWant: %v", g, w)
			}

			fromStatement := fmt.Sprintf("SELECT * FROM %s", quoted)
			convertedFromSQL, fromParams, _, fromErr := statementParser.ParseParameters(fromStatement)
			if fromErr != nil {
				t.Errorf("StatementParser rejected FROM clause: %s (err: %v)", fromStatement, fromErr)
			}
			if g, w := len(fromParams), 0; g != w {
				t.Errorf("unexpected parameter count for FROM clause %q\nGot:  %v\nWant: %v", fromStatement, g, w)
			}
			if g, w := convertedFromSQL, fromStatement; g != w {
				t.Errorf("FROM statement mutated by parser\nGot:  %v\nWant: %v", g, w)
			}

			// spansql.parsePathExp does not yet implement table.* syntax; verify the quoted prefix.
			spansqlTarget := strings.TrimSuffix(quoted, ".*")
			query, parseErr := spansql.ParseQuery(fmt.Sprintf("SELECT %s FROM test_table", spansqlTarget))
			if parseErr != nil {
				t.Fatalf("spansql.ParseQuery failed on %s: %v", sqlStatement, parseErr)
			}
			if g, w := len(query.Select.List), 1; g != w {
				t.Errorf("unexpected SELECT list count for %s\nGot:  %v\nWant: %v", sqlStatement, g, w)
			}
			if query.Select.Where != nil {
				t.Errorf("unexpected WHERE clause injected in %s: %v", sqlStatement, query.Select.Where)
			}

			// spansql's FROM parser only accepts single identifiers (not dotted schema.table paths).
			if !strings.Contains(spansqlTarget, "`.`") {
				tableQuery, tableParseErr := spansql.ParseQuery(fmt.Sprintf("SELECT * FROM %s", spansqlTarget))
				if tableParseErr != nil {
					t.Fatalf("spansql.ParseQuery failed in FROM clause for %s: %v", spansqlTarget, tableParseErr)
				}
				if g, w := len(tableQuery.Select.From), 1; g != w {
					t.Errorf("unexpected FROM count for %s\nGot:  %v\nWant: %v", spansqlTarget, g, w)
				}
				if tableQuery.Select.Where != nil {
					t.Errorf("unexpected WHERE clause injected in FROM query %s: %v", spansqlTarget, tableQuery.Select.Where)
				}
			}
		})
	}
}

func TestDialector_QuoteTo_GORMIntegration(t *testing.T) {
	dialector := Dialector{}
	db := &gorm.DB{Config: &gorm.Config{Dialector: dialector}}
	statement := &gorm.Statement{DB: db}

	var tableBuilder strings.Builder
	statement.QuoteTo(&tableBuilder, clause.Table{Name: "users"})
	if g, w := tableBuilder.String(), "`users`"; g != w {
		t.Errorf("table quote mismatch\nGot:  %v\nWant: %v", g, w)
	}

	var columnBuilder strings.Builder
	statement.QuoteTo(&columnBuilder, clause.Column{Table: "users", Name: "name"})
	if g, w := columnBuilder.String(), "`users`.`name`"; g != w {
		t.Errorf("column quote mismatch\nGot:  %v\nWant: %v", g, w)
	}

	var dottedColumnBuilder strings.Builder
	statement.QuoteTo(&dottedColumnBuilder, "users.name")
	if g, w := dottedColumnBuilder.String(), "`users`.`name`"; g != w {
		t.Errorf("dotted column quote mismatch\nGot:  %v\nWant: %v", g, w)
	}

	var injectedTableBuilder strings.Builder
	statement.QuoteTo(&injectedTableBuilder, clause.Table{Name: "users`; DROP TABLE users; --"})
	if g, w := injectedTableBuilder.String(), "`users\\`; DROP TABLE users; --`"; g != w {
		t.Errorf("injected table quote mismatch\nGot:  %v\nWant: %v", g, w)
	}

	var injectedColumnBuilder strings.Builder
	statement.QuoteTo(&injectedColumnBuilder, clause.Column{Table: "users\\", Name: "id` OR 1=1"})
	if g, w := injectedColumnBuilder.String(), "`users\\\\`.`id\\` OR 1=1`"; g != w {
		t.Errorf("injected column quote mismatch\nGot:  %v\nWant: %v", g, w)
	}
}
