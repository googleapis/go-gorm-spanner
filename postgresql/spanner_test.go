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
	"fmt"
	"slices"
	"testing"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/go-sql-spanner/testutil"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

func setupPGTestGormConnection(t *testing.T) (db *gorm.DB, server *testutil.MockedSpannerInMemTestServer, teardown func()) {
	t.Helper()
	server, _, serverTeardown := setupMockedTestServer(t)
	server.SetupSelectDialectResult(databasepb.DatabaseDialect_POSTGRESQL)
	return setupTestGormConnectionWithDialector(t, server, serverTeardown, New(postgres.Config{
		DSN:                  fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true", server.Address),
		PreferSimpleProtocol: true,
	}))
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

type compositePKUser struct {
	TenantID int64 `gorm:"primaryKey"`
	ID       int64 `gorm:"primaryKey"`
	Name     string
	Role     string
}

func TestBeforeUpdate_PreservesOmit(t *testing.T) {
	t.Parallel()

	wantSQL := `UPDATE "account_users" SET "name"=$1 WHERE "id" = $2`

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
			db, server, teardown := setupPGTestGormConnection(t)
			defer teardown()
			getExecuteSqlRequests(server)

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

func TestBeforeUpdate_CompositePrimaryKey(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupPGTestGormConnection(t)
	defer teardown()
	getExecuteSqlRequests(server)

	wantSQL := `UPDATE "composite_pk_users" SET "name"=$1 WHERE "tenant_id" = $2 AND "id" = $3`
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
			db, server, teardown := setupPGTestGormConnection(t)
			defer teardown()
			getExecuteSqlRequests(server)

			if err := tc.run(db); err != nil {
				t.Fatalf("update failed: %v", err)
			}

			execReqs := getExecuteSqlRequests(server)
			if g, w := len(execReqs), 0; g != w {
				t.Errorf("execute request count mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestBeforeUpdate_OmitAssociations(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupPGTestGormConnection(t)
	defer teardown()
	getExecuteSqlRequests(server)

	wantSQL := `UPDATE "parent_records" SET "name"=$1 WHERE "id" = $2`
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

	execReqs := getExecuteSqlRequests(server)
	if g, w := len(execReqs), 1; g != w {
		t.Fatalf("execute request count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := execReqs[0].Sql, wantSQL; g != w {
		t.Errorf("SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_ModelHookOmit(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupPGTestGormConnection(t)
	defer teardown()
	getExecuteSqlRequests(server)

	wantSQL := `UPDATE "hook_users" SET "name"=$1 WHERE "id" = $2`
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
			wantSQL: `UPDATE "account_users" SET "name"=$1,"role"=$2,"is_admin"=$3 WHERE "id" = $4`,
		},
		{
			name:    "Select explicitly includes primary key",
			selects: []string{"id", "name"},
			wantSQL: `UPDATE "account_users" SET "name"=$1 WHERE "id" = $2`,
		},
		{
			name:    "Select wildcard with Omit",
			selects: []string{"*"},
			omits:   []string{"role"},
			wantSQL: `UPDATE "account_users" SET "name"=$1,"is_admin"=$2 WHERE "id" = $3`,
		},
		{
			name:    "Select columns with Omit",
			selects: []string{"id", "name", "role"},
			omits:   []string{"role"},
			wantSQL: `UPDATE "account_users" SET "name"=$1 WHERE "id" = $2`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, server, teardown := setupPGTestGormConnection(t)
			defer teardown()
			getExecuteSqlRequests(server)

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

	db, server, teardown := setupPGTestGormConnection(t)
	defer teardown()
	getExecuteSqlRequests(server)

	updateSQL := `UPDATE "account_users" SET "name"=$1,"is_admin"=$2 WHERE "id" = $3`
	insertSQL := `INSERT INTO "account_users" ("name","is_admin","id") VALUES ($1,$2,$3) ON CONFLICT ("id") DO UPDATE SET "name"="excluded"."name","is_admin"="excluded"."is_admin" RETURNING "id"`
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

	execReqs := getExecuteSqlRequests(server)
	if g, w := len(execReqs), 2; g != w {
		t.Fatalf("execute request count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := execReqs[0].Sql, updateSQL; g != w {
		t.Errorf("update SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
	if g, w := execReqs[1].Sql, insertSQL; g != w {
		t.Errorf("fallback insert SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_SessionReusePreservesBaseOmits(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupPGTestGormConnection(t)
	defer teardown()
	getExecuteSqlRequests(server)

	updateSQL := `UPDATE "account_users" SET "name"=$1 WHERE "id" = $2`
	insertSQL := `INSERT INTO "account_users" ("name","is_admin","id") VALUES ($1,$2,$3) RETURNING "id"`
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
	if g, w := getLastSqlRequest(server).Sql, updateSQL; g != w {
		t.Errorf("update SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}

	created := accountUser{ID: 42, Name: "bob", Role: "admin", IsAdmin: true}
	if err := base.Create(&created).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if g, w := getLastSqlRequest(server).Sql, insertSQL; g != w {
		t.Errorf("create SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBeforeUpdate_TableWithoutSchema(t *testing.T) {
	t.Parallel()

	db, server, teardown := setupPGTestGormConnection(t)
	defer teardown()
	getExecuteSqlRequests(server)

	wantSQL := `UPDATE "account_users" SET "name"=$1 WHERE id = $2`
	if err := server.TestSpanner.PutStatementResult(wantSQL, &testutil.StatementResult{
		Type:        testutil.StatementResultUpdateCount,
		UpdateCount: 1,
	}); err != nil {
		t.Fatalf("failed to register statement result: %v", err)
	}

	if err := db.Table("account_users").Where("id = ?", 1).Update("name", "alice").Error; err != nil {
		t.Fatalf("table update failed: %v", err)
	}
	if g, w := getLastSqlRequest(server).Sql, wantSQL; g != w {
		t.Errorf("table Update SQL mismatch\nGot:  %v\nWant: %v", g, w)
	}

	if err := db.Table("account_users").Omit("role").Where("id = ?", 1).Updates(map[string]interface{}{
		"name": "alice",
		"role": "admin",
	}).Error; err != nil {
		t.Fatalf("table Updates with Omit failed: %v", err)
	}
	if g, w := getLastSqlRequest(server).Sql, wantSQL; g != w {
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

func getExecuteSqlRequests(server *testutil.MockedSpannerInMemTestServer) []*spannerpb.ExecuteSqlRequest {
	var execReqs []*spannerpb.ExecuteSqlRequest
loop:
	for {
		select {
		case req := <-server.TestSpanner.ReceivedRequests():
			if sqlReq, ok := req.(*spannerpb.ExecuteSqlRequest); ok {
				execReqs = append(execReqs, sqlReq)
			}
		default:
			break loop
		}
	}
	return execReqs
}

func getLastSqlRequest(server *testutil.MockedSpannerInMemTestServer) *spannerpb.ExecuteSqlRequest {
	execReqs := getExecuteSqlRequests(server)
	if len(execReqs) > 0 {
		return execReqs[len(execReqs)-1]
	}
	return &spannerpb.ExecuteSqlRequest{}
}
