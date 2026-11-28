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
	"slices"
	"strconv"
	"testing"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	spannerdriver "github.com/googleapis/go-sql-spanner"
	"github.com/googleapis/go-sql-spanner/testutil"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
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

type accountUser struct {
	ID      int64 `gorm:"primaryKey"`
	Name    string
	Role    string
	IsAdmin bool
}

type hookUser struct {
	ID   int64 `gorm:"primaryKey"`
	Name string
	Role string
}

func (*hookUser) BeforeUpdate(tx *gorm.DB) error {
	tx.Statement.Omit("role")
	return nil
}

type parentRecord struct {
	ID       int64 `gorm:"primaryKey"`
	Name     string
	Children []childRecord `gorm:"foreignKey:ParentID"`
}

type childRecord struct {
	ID       int64 `gorm:"primaryKey"`
	Name     string
	ParentID int64
}

func TestBeforeUpdate_PreservesOmit(t *testing.T) {
	t.Parallel()

	wantSQL := "UPDATE `account_users` SET `name`=@p1 WHERE `id` = @p2"

	tests := []struct {
		name string
		run  func(db *gorm.DB) error
	}{
		{
			name: "map update with multi-arg Omit",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Omit("role", "is_admin").Updates(map[string]interface{}{
					"id":       999,
					"name":     "alice",
					"role":     "admin",
					"is_admin": true,
				}).Error
			},
		},
		{
			name: "struct update with multi-arg Omit",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Omit("role", "is_admin").Updates(accountUser{
					ID:      999,
					Name:    "alice",
					Role:    "admin",
					IsAdmin: true,
				}).Error
			},
		},
		{
			name: "map update with comma-separated Omit",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Omit("role,is_admin").Updates(map[string]interface{}{
					"id":       999,
					"name":     "alice",
					"role":     "admin",
					"is_admin": true,
				}).Error
			},
		},
		{
			name: "map update with Go field names in Omit",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Omit("Role", "IsAdmin").Updates(map[string]interface{}{
					"id":       999,
					"name":     "alice",
					"role":     "admin",
					"is_admin": true,
				}).Error
			},
		},
		{
			name: "map update when Omit already includes primary key",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Omit("id", "role", "is_admin").Updates(map[string]interface{}{
					"id":       999,
					"name":     "alice",
					"role":     "admin",
					"is_admin": true,
				}).Error
			},
		},
		{
			name: "UpdateColumns with Omit",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Omit("role", "is_admin").UpdateColumns(map[string]interface{}{
					"id":       999,
					"name":     "alice",
					"role":     "admin",
					"is_admin": true,
				}).Error
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, server, teardown := setupTestGormConnection(t)
			defer teardown()

			if err := server.TestSpanner.PutStatementResult(wantSQL, &testutil.StatementResult{
				Type:        testutil.StatementResultUpdateCount,
				UpdateCount: 1,
			}); err != nil {
				t.Fatalf("failed to register statement result: %v", err)
			}

			if err := tc.run(db); err != nil {
				t.Fatalf("update failed: %v", err)
			}

			req := getLastSqlRequest(server)
			if g, w := req.Sql, wantSQL; g != w {
				t.Errorf("SQL mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := len(req.Params.Fields), 2; g != w {
				t.Errorf("param count mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := req.Params.Fields["p1"].GetStringValue(), "alice"; g != w {
				t.Errorf("name param mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := req.Params.Fields["p2"].GetStringValue(), "1"; g != w {
				t.Errorf("where id param mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

type compositePKUser struct {
	TenantID int64 `gorm:"primaryKey"`
	ID       int64 `gorm:"primaryKey"`
	Name     string
	Role     string
}

func TestBeforeUpdate_CompositePrimaryKey(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	wantSQL := "UPDATE `composite_pk_users` SET `name`=@p1 WHERE `tenant_id` = @p2 AND `id` = @p3"
	if err := server.TestSpanner.PutStatementResult(wantSQL, &testutil.StatementResult{
		Type:        testutil.StatementResultUpdateCount,
		UpdateCount: 1,
	}); err != nil {
		t.Fatalf("failed to register statement result: %v", err)
	}

	u := compositePKUser{TenantID: 10, ID: 1}
	if err := db.Model(&u).Omit("role").Updates(map[string]interface{}{
		"tenant_id": 999,
		"id":        999,
		"name":      "alice",
		"role":      "admin",
	}).Error; err != nil {
		t.Fatalf("update failed: %v", err)
	}

	req := getLastSqlRequest(server)
	if g, w := req.Sql, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := len(req.Params.Fields), 3; g != w {
		t.Errorf("param count mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_NoOpUpdates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(db *gorm.DB) error
	}{
		{
			name: "single omitted column update",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Omit("role").Update("role", "admin").Error
			},
		},
		{
			name: "direct primary key update",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Update("id", 999).Error
			},
		},
		{
			name: "map update with only primary key and omitted columns",
			run: func(db *gorm.DB) error {
				u := accountUser{ID: 1}
				return db.Model(&u).Omit("role").Updates(map[string]interface{}{
					"id":   999,
					"role": "admin",
				}).Error
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, server, teardown := setupTestGormConnection(t)
			defer teardown()
			drainRequestsFromServer(server.TestSpanner)

			if err := tc.run(db); err != nil {
				t.Fatalf("update failed: %v", err)
			}

			reqs := drainRequestsFromServer(server.TestSpanner)
			execReqs := requestsOfType(reqs, reflect.TypeOf(&spannerpb.ExecuteSqlRequest{}))
			if g, w := len(execReqs), 0; g != w {
				t.Errorf("execute request count mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestBeforeUpdate_OmitAssociations(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()
	drainRequestsFromServer(server.TestSpanner)

	wantSQL := "UPDATE `parent_records` SET `name`=@p1 WHERE `id` = @p2"
	if err := server.TestSpanner.PutStatementResult(wantSQL, &testutil.StatementResult{
		Type:        testutil.StatementResultUpdateCount,
		UpdateCount: 1,
	}); err != nil {
		t.Fatalf("failed to register statement result: %v", err)
	}

	p := parentRecord{
		ID:       1,
		Name:     "updated",
		Children: []childRecord{{ID: 10, Name: "child"}},
	}
	if err := db.Omit(clause.Associations).Updates(&p).Error; err != nil {
		t.Fatalf("update failed: %v", err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	execReqs := requestsOfType(reqs, reflect.TypeOf(&spannerpb.ExecuteSqlRequest{}))
	if g, w := len(execReqs), 1; g != w {
		t.Fatalf("execute request count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := execReqs[0].(*spannerpb.ExecuteSqlRequest).Sql, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_ModelHookOmit(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	wantSQL := "UPDATE `hook_users` SET `name`=@p1 WHERE `id` = @p2"
	if err := server.TestSpanner.PutStatementResult(wantSQL, &testutil.StatementResult{
		Type:        testutil.StatementResultUpdateCount,
		UpdateCount: 1,
	}); err != nil {
		t.Fatalf("failed to register statement result: %v", err)
	}

	u := hookUser{ID: 1}
	if err := db.Model(&u).Updates(map[string]interface{}{
		"id":   999,
		"name": "alice",
		"role": "admin",
	}).Error; err != nil {
		t.Fatalf("update failed: %v", err)
	}

	req := getLastSqlRequest(server)
	if g, w := req.Sql, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_SelectStillOmitsPrimaryKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		selects []string
		omits   []string
		wantSQL string
	}{
		{
			name:    "Select wildcard",
			selects: []string{"*"},
			wantSQL: "UPDATE `account_users` SET `name`=@p1,`role`=@p2,`is_admin`=@p3 WHERE `id` = @p4",
		},
		{
			name:    "Select explicitly includes primary key",
			selects: []string{"id", "name"},
			wantSQL: "UPDATE `account_users` SET `name`=@p1 WHERE `id` = @p2",
		},
		{
			name:    "Select wildcard with Omit",
			selects: []string{"*"},
			omits:   []string{"role"},
			wantSQL: "UPDATE `account_users` SET `name`=@p1,`is_admin`=@p2 WHERE `id` = @p3",
		},
		{
			name:    "Select columns with Omit",
			selects: []string{"id", "name", "role"},
			omits:   []string{"role"},
			wantSQL: "UPDATE `account_users` SET `name`=@p1 WHERE `id` = @p2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, server, teardown := setupTestGormConnection(t)
			defer teardown()

			if err := server.TestSpanner.PutStatementResult(tc.wantSQL, &testutil.StatementResult{
				Type:        testutil.StatementResultUpdateCount,
				UpdateCount: 1,
			}); err != nil {
				t.Fatalf("failed to register statement result: %v", err)
			}

			u := accountUser{ID: 1}
			tx := db.Model(&u).Select(tc.selects)
			if len(tc.omits) > 0 {
				tx = tx.Omit(tc.omits...)
			}
			if err := tx.Updates(accountUser{ID: 999, Name: "alice", Role: "member", IsAdmin: false}).Error; err != nil {
				t.Fatalf("update failed: %v", err)
			}

			req := getLastSqlRequest(server)
			if g, w := req.Sql, tc.wantSQL; g != w {
				t.Errorf("SQL mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestBeforeUpdate_SaveWithOmit(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()
	drainRequestsFromServer(server.TestSpanner)

	updateSQL := "UPDATE `account_users` SET `name`=@p1,`is_admin`=@p2 WHERE `id` = @p3"
	insertSQL := "INSERT OR UPDATE INTO `account_users` (`name`,`is_admin`,`id`) VALUES (@p1,@p2,@p3) THEN RETURN `id`"
	if err := server.TestSpanner.PutStatementResult(updateSQL, &testutil.StatementResult{
		Type:        testutil.StatementResultUpdateCount,
		UpdateCount: 0,
	}); err != nil {
		t.Fatalf("failed to register update result: %v", err)
	}
	if err := server.TestSpanner.PutStatementResult(insertSQL, &testutil.StatementResult{
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
					{Kind: &structpb.Value_StringValue{StringValue: "42"}},
				}},
			},
		},
	}); err != nil {
		t.Fatalf("failed to register insert result: %v", err)
	}

	u := accountUser{ID: 42, Name: "alice", Role: "admin", IsAdmin: true}
	if err := db.Omit("role").Save(&u).Error; err != nil {
		t.Fatalf("save failed: %v", err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	execReqs := requestsOfType(reqs, reflect.TypeOf(&spannerpb.ExecuteSqlRequest{}))
	if g, w := len(execReqs), 2; g != w {
		t.Fatalf("execute request count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := execReqs[0].(*spannerpb.ExecuteSqlRequest).Sql, updateSQL; g != w {
		t.Errorf("update SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := execReqs[1].(*spannerpb.ExecuteSqlRequest).Sql, insertSQL; g != w {
		t.Errorf("fallback insert SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_SessionReusePreservesBaseOmits(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	updateSQL := "UPDATE `account_users` SET `name`=@p1 WHERE `id` = @p2"
	insertSQL := "INSERT INTO `account_users` (`name`,`is_admin`,`id`) VALUES (@p1,@p2,@p3) THEN RETURN `id`"
	if err := server.TestSpanner.PutStatementResult(updateSQL, &testutil.StatementResult{
		Type:        testutil.StatementResultUpdateCount,
		UpdateCount: 1,
	}); err != nil {
		t.Fatalf("failed to register update result: %v", err)
	}
	if err := server.TestSpanner.PutStatementResult(insertSQL, &testutil.StatementResult{
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
					{Kind: &structpb.Value_StringValue{StringValue: "42"}},
				}},
			},
		},
	}); err != nil {
		t.Fatalf("failed to register insert result: %v", err)
	}

	base := db.Omit("role").Session(&gorm.Session{})
	existing := accountUser{ID: 1}
	if err := base.Model(&existing).Updates(map[string]interface{}{
		"id":   999,
		"name": "alice",
		"role": "admin",
	}).Error; err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if g, w := getLastSql(server), updateSQL; g != w {
		t.Errorf("update SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}

	// Reusing base session for Create must still include primary key `id` while omitting `role`.
	created := accountUser{ID: 42, Name: "bob", Role: "admin", IsAdmin: true}
	if err := base.Create(&created).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if g, w := getLastSql(server), insertSQL; g != w {
		t.Errorf("create SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_TableWithoutSchema(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupTestGormConnection(t)
	defer teardown()

	wantSQL := "UPDATE `account_users` SET `name`=@p1 WHERE id = @p2"
	if err := server.TestSpanner.PutStatementResult(wantSQL, &testutil.StatementResult{
		Type:        testutil.StatementResultUpdateCount,
		UpdateCount: 1,
	}); err != nil {
		t.Fatalf("failed to register statement result: %v", err)
	}

	if err := db.Table("account_users").Where("id = ?", 1).Update("name", "alice").Error; err != nil {
		t.Fatalf("table update failed: %v", err)
	}
	if g, w := getLastSql(server), wantSQL; g != w {
		t.Errorf("table Update SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}

	if err := db.Table("account_users").Omit("role").Where("id = ?", 1).Updates(map[string]interface{}{
		"name": "alice",
		"role": "admin",
	}).Error; err != nil {
		t.Fatalf("table Updates with Omit failed: %v", err)
	}
	if g, w := getLastSql(server), wantSQL; g != w {
		t.Errorf("table Updates SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_DelegatesToInternal(t *testing.T) {
	t.Parallel()

	BeforeUpdate(nil)
	stmt := &gorm.Statement{
		Omits: []string{"role"},
		Schema: &schema.Schema{
			PrimaryFieldDBNames: []string{"id"},
		},
	}
	BeforeUpdate(&gorm.DB{Statement: stmt})
	if g, w := stmt.Omits, []string{"role", "id"}; !slices.Equal(g, w) {
		t.Errorf("Omits mismatch\nGot:  %v\nWant: %v", g, w)
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
