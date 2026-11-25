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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

func TestCreateTable_AutoAddPrimaryKey_DoesNotMutateSchema(t *testing.T) {
	t.Parallel()

	type fkParentModel struct {
		ID uint
	}
	type noPKWithIndexModel struct {
		Name  string `gorm:"index:idx_no_pk_name"`
		Count int    `gorm:"check:chk_count_positive,count > 0"`
	}
	type noPKWithConstraintsModel struct {
		First    string `gorm:"index:idx_first"`
		Second   string `gorm:"index:idx_second;unique"`
		ParentID uint
		Parent   *fkParentModel
	}
	type explicitTypePKModel struct {
		CustomID int64 `gorm:"type:bigint primary key"`
		Label    string
	}
	type standardPKModel struct {
		ID    uint
		Title string
	}

	tests := []struct {
		name           string
		model          interface{}
		tableOptions   string
		wantStatements []string
		wantDBNames    []string
		wantFieldCount int
	}{
		{
			name:  "model without primary key emits generated primary key",
			model: &noPKWithIndexModel{},
			wantStatements: []string{
				`CREATE TABLE "no_pk_with_index_models" ("name" text,"count" int,"spanner_gorm_generated_id" serial primary key,CONSTRAINT "chk_count_positive" CHECK (count > 0))`,
				`CREATE INDEX IF NOT EXISTS "idx_no_pk_name" ON "no_pk_with_index_models" ("name")`,
			},
			wantDBNames:    []string{"name", "count"},
			wantFieldCount: 2,
		},
		{
			name:         "model without primary key emits FK, unique constraint, multiple indexes, and table options",
			model:        &noPKWithConstraintsModel{},
			tableOptions: " /* test_table_option */",
			wantStatements: []string{
				`CREATE TABLE "no_pk_with_constraints_models" ("first" text,"second" text,"parent_id" int,"spanner_gorm_generated_id" serial primary key,CONSTRAINT "fk_no_pk_with_constraints_models_parent" FOREIGN KEY ("parent_id") REFERENCES "fk_parent_models"("id"),CONSTRAINT "uni_no_pk_with_constraints_models_second" UNIQUE ("second")) /* test_table_option */`,
				`CREATE INDEX IF NOT EXISTS "idx_first" ON "no_pk_with_constraints_models" ("first")`,
				`CREATE INDEX IF NOT EXISTS "idx_second" ON "no_pk_with_constraints_models" ("second")`,
			},
			wantDBNames:    []string{"first", "second", "parent_id"},
			wantFieldCount: 4, // includes the Parent relationship field
		},
		{
			name:  "model with primary key in data type does not emit generated primary key",
			model: &explicitTypePKModel{},
			wantStatements: []string{
				`CREATE TABLE "explicit_type_pk_models" ("custom_id" bigint primary key,"label" text)`,
			},
			wantDBNames:    []string{"custom_id", "label"},
			wantFieldCount: 2,
		},
		{
			name:  "model with standard primary key does not emit generated primary key",
			model: &standardPKModel{},
			wantStatements: []string{
				`CREATE TABLE "standard_pk_models" ("id" serial,"title" text,PRIMARY KEY ("id"))`,
			},
			wantDBNames:    []string{"id", "title"},
			wantFieldCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server, _, serverTeardown := setupMockedTestServer(t)
			anyProto, err := anypb.New(&emptypb.Empty{})
			if err != nil {
				serverTeardown()
				t.Fatalf("failed to marshal empty proto: %v", err)
			}
			resps := make([]proto.Message, len(tc.wantStatements))
			for i := range resps {
				resps[i] = &longrunningpb.Operation{
					Name:   fmt.Sprintf("test-operation-%d", i),
					Done:   true,
					Result: &longrunningpb.Operation_Response{Response: anyProto},
				}
			}
			server.TestDatabaseAdmin.SetResps(resps)

			dialector := NewWithSpannerConfig(postgres.Config{
				DSN: fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true", server.Address),
			}, SpannerConfig{
				AutoAddPrimaryKey: true,
			})
			db, server, teardown := setupTestGormConnectionWithDialector(t, server, serverTeardown, dialector)
			defer teardown()

			migratorDB := db.Session(&gorm.Session{})
			if tc.tableOptions != "" {
				migratorDB = migratorDB.Set("gorm:table_options", tc.tableOptions)
			}
			if err := migratorDB.Migrator().CreateTable(tc.model); err != nil {
				t.Fatalf("CreateTable failed: %v", err)
			}

			requests := server.TestDatabaseAdmin.Reqs()
			var gotStatements []string
			for _, req := range requests {
				ddlReq, ok := req.(*databasepb.UpdateDatabaseDdlRequest)
				if !ok {
					t.Fatalf("unexpected admin request type: %T", req)
				}
				gotStatements = append(gotStatements, ddlReq.GetStatements()...)
			}
			if g, w := gotStatements, tc.wantStatements; !slices.Equal(g, w) {
				t.Errorf("DDL statements mismatch\nGot:  %v\nWant: %v", g, w)
			}

			stmt := &gorm.Statement{DB: db}
			if err := stmt.Parse(tc.model); err != nil {
				t.Fatalf("failed to parse schema: %v", err)
			}
			if g, w := stmt.Schema.DBNames, tc.wantDBNames; !slices.Equal(g, w) {
				t.Errorf("stmt.Schema.DBNames mutated\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := len(stmt.Schema.Fields), tc.wantFieldCount; g != w {
				t.Errorf("stmt.Schema.Fields length mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if _, ok := stmt.Schema.FieldsByDBName[generatedPrimaryKeyColumn]; ok {
				t.Errorf("stmt.Schema.FieldsByDBName unexpectedly contains %q", generatedPrimaryKeyColumn)
			}
			if _, ok := stmt.Schema.FieldsByName[generatedPrimaryKeyColumn]; ok {
				t.Errorf("stmt.Schema.FieldsByName unexpectedly contains %q", generatedPrimaryKeyColumn)
			}
		})
	}
}

func TestCreateTable_AutoAddPrimaryKey_Concurrent(t *testing.T) {
	t.Parallel()

	type concurrentNoPKModel struct {
		Name  string
		Value int64
	}

	server, _, serverTeardown := setupMockedTestServer(t)
	dialector := NewWithSpannerConfig(postgres.Config{
		DSN: fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true", server.Address),
	}, SpannerConfig{
		AutoAddPrimaryKey: true,
	})
	db, _, teardown := setupTestGormConnectionWithDialector(t, server, serverTeardown, dialector)
	defer teardown()

	// Pre-populate the schema cache on this *gorm.DB before launching concurrent readers and migrators.
	initStmt := &gorm.Statement{DB: db}
	if err := initStmt.Parse(&concurrentNoPKModel{}); err != nil {
		t.Fatalf("initial schema parse failed: %v", err)
	}

	enteredCreateTable := make(chan struct{})
	releaseCreateTable := make(chan struct{})
	var paused atomic.Bool
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseCreateTable) })
	}
	defer release()

	var mu sync.Mutex
	var capturedSQL []string
	if err := db.Callback().Raw().Before("gorm:raw").Register("test:pause_create_table", func(tx *gorm.DB) {
		sqlText := tx.Statement.SQL.String()
		mu.Lock()
		capturedSQL = append(capturedSQL, sqlText)
		mu.Unlock()
		if strings.HasPrefix(sqlText, "CREATE TABLE ") && paused.CompareAndSwap(false, true) {
			close(enteredCreateTable)
			<-releaseCreateTable
		}
	}); err != nil {
		t.Fatalf("failed to register callback: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- db.Session(&gorm.Session{DryRun: true, NewDB: true}).Migrator().CreateTable(&concurrentNoPKModel{})
	}()

	// Deterministically pause the first CreateTable mid-flight at CREATE TABLE.
	select {
	case <-enteredCreateTable:
	case err := <-errCh:
		t.Fatalf("first CreateTable failed before CREATE TABLE: %v", err)
	}

	// Verify the cached schema was not mutated while CreateTable is in flight.
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&concurrentNoPKModel{}); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if field := stmt.Schema.LookUpField("name"); field == nil {
		t.Fatalf("LookUpField(name) returned nil")
	}
	if _, ok := stmt.Schema.FieldsByDBName[generatedPrimaryKeyColumn]; ok {
		t.Errorf("FieldsByDBName unexpectedly mutated with %q", generatedPrimaryKeyColumn)
	}
	if _, ok := stmt.Schema.FieldsByName[generatedPrimaryKeyColumn]; ok {
		t.Errorf("FieldsByName unexpectedly mutated with %q", generatedPrimaryKeyColumn)
	}
	if g, w := stmt.Schema.DBNames, []string{"name", "value"}; !slices.Equal(g, w) {
		t.Errorf("stmt.Schema.DBNames mismatch\nGot:  %v\nWant: %v", g, w)
	}

	// Verify concurrent DML statement building and a second CreateTable while the first is in flight.
	createStmt := db.Session(&gorm.Session{DryRun: true, NewDB: true}).Clauses(clause.OnConflict{DoNothing: true}).Create(&concurrentNoPKModel{Name: "test", Value: 1}).Statement
	if g, w := createStmt.SQL.String(), `ON CONFLICT ("spanner_gorm_generated_id") DO NOTHING`; !strings.Contains(g, w) {
		t.Errorf("OnConflict SQL mismatch\nGot:  %s\nWant substring: %s", g, w)
	}
	if err := db.Session(&gorm.Session{DryRun: true, NewDB: true}).Migrator().CreateTable(&concurrentNoPKModel{}); err != nil {
		t.Fatalf("concurrent CreateTable failed: %v", err)
	}

	release()
	if err := <-errCh; err != nil {
		t.Fatalf("first CreateTable failed: %v", err)
	}

	wantTableSQL := `CREATE TABLE "concurrent_no_pk_models" ("name" text,"value" int,"spanner_gorm_generated_id" serial primary key)`
	mu.Lock()
	gotSQL := slices.Clone(capturedSQL)
	mu.Unlock()
	if g, w := gotSQL, []string{wantTableSQL, wantTableSQL}; !slices.Equal(g, w) {
		t.Errorf("DDL statements mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestCreateTable_AutoAddPrimaryKey_MixedModels(t *testing.T) {
	t.Parallel()

	type mixedPKModel struct {
		ID   uint
		Name string
	}
	type mixedNoPKModel struct {
		Label string
	}

	server, _, serverTeardown := setupMockedTestServer(t)
	anyProto, err := anypb.New(&emptypb.Empty{})
	if err != nil {
		serverTeardown()
		t.Fatalf("failed to marshal empty proto: %v", err)
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

	dialector := NewWithSpannerConfig(postgres.Config{
		DSN: fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true", server.Address),
	}, SpannerConfig{
		AutoAddPrimaryKey: true,
	})
	db, server, teardown := setupTestGormConnectionWithDialector(t, server, serverTeardown, dialector)
	defer teardown()

	if err := db.Session(&gorm.Session{}).Migrator().CreateTable(&mixedPKModel{}, &mixedNoPKModel{}); err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	var gotStatements []string
	for _, req := range server.TestDatabaseAdmin.Reqs() {
		ddlReq, ok := req.(*databasepb.UpdateDatabaseDdlRequest)
		if !ok {
			t.Fatalf("unexpected admin request type: %T", req)
		}
		gotStatements = append(gotStatements, ddlReq.GetStatements()...)
	}
	wantStatements := []string{
		`CREATE TABLE "mixed_pk_models" ("id" serial,"name" text,PRIMARY KEY ("id"))`,
		`CREATE TABLE "mixed_no_pk_models" ("label" text,"spanner_gorm_generated_id" serial primary key)`,
	}
	if g, w := gotStatements, wantStatements; !slices.Equal(g, w) {
		t.Errorf("DDL statements mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestClauseOnConflict_AutoAddPrimaryKey(t *testing.T) {
	t.Parallel()

	type noPKModel struct {
		Name string
	}
	type explicitTypePKModel struct {
		CustomID int64 `gorm:"type:bigint primary key"`
		Name     string
	}
	type serialPKModel struct {
		CustomID int64 `gorm:"type:serial primary key"`
		Name     string
	}
	type standardPKModel struct {
		ID   uint
		Name string
	}

	tests := []struct {
		name              string
		autoAddPrimaryKey bool
		model             interface{}
		wantSQLFragment   string
		wantAbsent        string
	}{
		{
			name:              "autoAddPrimaryKey enabled without PK uses generated PK column",
			autoAddPrimaryKey: true,
			model:             &noPKModel{Name: "a"},
			wantSQLFragment:   `ON CONFLICT ("spanner_gorm_generated_id") DO NOTHING`,
		},
		{
			name:              "autoAddPrimaryKey enabled with explicit primary key in data type uses custom PK",
			autoAddPrimaryKey: true,
			model:             &explicitTypePKModel{CustomID: 1, Name: "a"},
			wantSQLFragment:   `ON CONFLICT ("custom_id") DO NOTHING`,
		},
		{
			name:              "autoAddPrimaryKey disabled with serial primary key in data type uses custom PK",
			autoAddPrimaryKey: false,
			model:             &serialPKModel{CustomID: 1, Name: "a"},
			wantSQLFragment:   `ON CONFLICT ("custom_id") DO NOTHING`,
		},
		{
			name:              "autoAddPrimaryKey enabled with standard PK uses standard PK",
			autoAddPrimaryKey: true,
			model:             &standardPKModel{ID: 1, Name: "a"},
			wantSQLFragment:   `ON CONFLICT ("id") DO NOTHING`,
		},
		{
			name:              "autoAddPrimaryKey disabled without PK does not add generated PK column",
			autoAddPrimaryKey: false,
			model:             &noPKModel{Name: "a"},
			wantSQLFragment:   `ON CONFLICT DO NOTHING`,
			wantAbsent:        generatedPrimaryKeyColumn,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server, _, serverTeardown := setupMockedTestServer(t)
			dialector := NewWithSpannerConfig(postgres.Config{
				DSN: fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true", server.Address),
			}, SpannerConfig{
				AutoAddPrimaryKey: tc.autoAddPrimaryKey,
			})
			db, _, teardown := setupTestGormConnectionWithDialector(t, server, serverTeardown, dialector)
			defer teardown()

			stmt := db.Session(&gorm.Session{DryRun: true}).Clauses(clause.OnConflict{DoNothing: true}).Create(tc.model).Statement
			if g, w := stmt.SQL.String(), tc.wantSQLFragment; !strings.Contains(g, w) {
				t.Errorf("SQL mismatch\nGot:  %s\nWant substring: %s", g, w)
			}
			if g := stmt.SQL.String(); tc.wantAbsent != "" && strings.Contains(g, tc.wantAbsent) {
				t.Errorf("SQL unexpectedly contained %q: %s", tc.wantAbsent, g)
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
