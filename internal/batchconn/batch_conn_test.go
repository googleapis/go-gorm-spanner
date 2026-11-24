// Copyright 2026 Google LLC.
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

package batchconn

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"slices"
	"testing"

	spannerdriver "github.com/googleapis/go-sql-spanner"
	"github.com/googleapis/go-sql-spanner/testutil"
	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type testDialector struct {
	connPool gorm.ConnPool
}

func (d testDialector) Name() string { return "spanner" }
func (d testDialector) Initialize(db *gorm.DB) error {
	callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{})
	db.ConnPool = d.connPool
	return nil
}
func (d testDialector) Migrator(*gorm.DB) gorm.Migrator                       { return nil }
func (d testDialector) DataTypeOf(*schema.Field) string                       { return "" }
func (d testDialector) DefaultValueOf(*schema.Field) clause.Expression        { return clause.Expr{} }
func (d testDialector) BindVarTo(clause.Writer, *gorm.Statement, interface{}) {}
func (d testDialector) QuoteTo(clause.Writer, string)                         {}
func (d testDialector) Explain(sql string, vars ...interface{}) string        { return sql }

type mockConnPool struct {
	execQueries []string
}

func (m *mockConnPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (m *mockConnPool) ExecContext(_ context.Context, query string, _ ...interface{}) (sql.Result, error) {
	m.execQueries = append(m.execQueries, query)
	return driver.ResultNoRows, nil
}

func (m *mockConnPool) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("not implemented")
}

func (m *mockConnPool) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return &sql.Row{}
}

func setupMockDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	server, _, serverTeardown := testutil.NewMockedSpannerInMemTestServer(t)
	dsn := fmt.Sprintf("%s/projects/p/instances/i/databases/d?useplaintext=true", server.Address)
	sqlDB, err := sql.Open("spanner", dsn)
	if err != nil {
		serverTeardown()
		t.Fatalf("failed to open sql.DB: %v", err)
	}
	return sqlDB, func() {
		_ = sqlDB.Close()
		serverTeardown()
	}
}

func assertInDDLBatch(t *testing.T, conn *sql.Conn, want bool) {
	t.Helper()
	var inBatch bool
	if err := conn.Raw(func(driverConn any) error {
		spannerConn, ok := driverConn.(spannerdriver.SpannerConn)
		if !ok {
			return fmt.Errorf("expected SpannerConn, got %T", driverConn)
		}
		inBatch = spannerConn.InDDLBatch()
		return nil
	}); err != nil {
		t.Fatalf("conn.Raw failed: %v", err)
	}
	if g, w := inBatch, want; g != w {
		t.Errorf("InDDLBatch mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestBatchFunctions_InvalidDB(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		db   *gorm.DB
	}{
		{name: "nil db", db: nil},
		{name: "nil config", db: &gorm.DB{}},
		{name: "nil statement", db: &gorm.DB{Config: &gorm.Config{}}},
		{name: "nil conn pool", db: &gorm.DB{Config: &gorm.Config{}, Statement: &gorm.Statement{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if g, w := StartBatchDDL(tc.db), gorm.ErrInvalidDB; !errors.Is(g, w) {
				t.Errorf("StartBatchDDL error mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := RunBatch(tc.db), gorm.ErrInvalidDB; !errors.Is(g, w) {
				t.Errorf("RunBatch error mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := AbortBatch(tc.db), gorm.ErrInvalidDB; !errors.Is(g, w) {
				t.Errorf("AbortBatch error mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if _, err := RunDDLBatch(tc.db, false, func(*gorm.DB) error { return nil }); !errors.Is(err, gorm.ErrInvalidDB) {
				t.Errorf("RunDDLBatch error mismatch\nGot:  %v\nWant: %v", err, gorm.ErrInvalidDB)
			}
		})
	}
}

func TestStartBatchDDL_PooledDBReturnsError(t *testing.T) {
	t.Parallel()

	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()

	gormDB, err := gorm.Open(testDialector{connPool: sqlDB}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open gorm DB: %v", err)
	}

	if err := StartBatchDDL(gormDB); !errors.Is(err, errDedicatedConnRequired) {
		t.Errorf("StartBatchDDL error mismatch\nGot:  %v\nWant: %v", err, errDedicatedConnRequired)
	}
}

func TestUnderlyingConn(t *testing.T) {
	t.Parallel()

	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()

	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to check out sql.Conn: %v", err)
	}
	defer func() { _ = conn.Close() }()

	var typedNilPreparedStmt *gorm.PreparedStmtDB
	tests := []struct {
		name     string
		db       *gorm.DB
		wantConn *sql.Conn
		wantOK   bool
	}{
		{
			name:   "nil db",
			db:     nil,
			wantOK: false,
		},
		{
			name:   "nil config",
			db:     &gorm.DB{},
			wantOK: false,
		},
		{
			name:   "typed-nil PreparedStmtDB",
			db:     &gorm.DB{Config: &gorm.Config{ConnPool: typedNilPreparedStmt}},
			wantOK: false,
		},
		{
			name:   "sql.DB pool",
			db:     &gorm.DB{Config: &gorm.Config{ConnPool: sqlDB}},
			wantOK: false,
		},
		{
			name:     "direct sql.Conn",
			db:       &gorm.DB{Config: &gorm.Config{ConnPool: conn}},
			wantConn: conn,
			wantOK:   true,
		},
		{
			name:     "PreparedStmtDB wrapping sql.Conn",
			db:       &gorm.DB{Config: &gorm.Config{ConnPool: gorm.NewPreparedStmtDB(conn, 10, 0)}},
			wantConn: conn,
			wantOK:   true,
		},
		{
			name: "Statement.ConnPool takes precedence",
			db: &gorm.DB{
				Config:    &gorm.Config{ConnPool: sqlDB},
				Statement: &gorm.Statement{ConnPool: conn},
			},
			wantConn: conn,
			wantOK:   true,
		},
		{
			name: "Statement.ConnPool non-conn overrides Config.ConnPool",
			db: &gorm.DB{
				Config:    &gorm.Config{ConnPool: conn},
				Statement: &gorm.Statement{ConnPool: sqlDB},
			},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotConn, gotOK := underlyingConn(tc.db)
			if g, w := gotOK, tc.wantOK; g != w {
				t.Errorf("underlyingConn ok mismatch\nGot:  %v\nWant: %v", g, w)
			}
			if g, w := gotConn, tc.wantConn; g != w {
				t.Errorf("underlyingConn conn mismatch\nGot:  %v\nWant: %v", g, w)
			}
		})
	}
}

func TestGetBatchedStatements_UnexpectedConnPool(t *testing.T) {
	t.Parallel()

	db := &gorm.DB{Config: &gorm.Config{}}
	if _, err := getBatchedStatements(db); err == nil {
		t.Fatalf("expected error for missing ConnPool, got nil")
	}
}

func TestRunDDLBatch_CancelledContextReleasesConn(t *testing.T) {
	t.Parallel()

	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()
	sqlDB.SetMaxOpenConns(1)

	gormDB, err := gorm.Open(testDialector{connPool: sqlDB}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open gorm DB: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, err = RunDDLBatch(gormDB.WithContext(ctx), false, func(tx *gorm.DB) error {
		if g, w := sqlDB.Stats().InUse, 1; g != w {
			t.Fatalf("InUse mismatch while batch active\nGot:  %v\nWant: %v", g, w)
		}
		cancel()
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunDDLBatch error mismatch\nGot:  %v\nWant: %v", err, context.Canceled)
	}
	if g, w := sqlDB.Stats().InUse, 0; g != w {
		t.Errorf("InUse mismatch after cancelled RunDDLBatch\nGot:  %v\nWant: %v", g, w)
	}
}

func TestRunDDLBatch_UserConn_AbortOnErrorAndCancelledContext(t *testing.T) {
	t.Parallel()

	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()

	userConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to check out sql.Conn: %v", err)
	}
	defer func() { _ = userConn.Close() }()

	gormDB, err := gorm.Open(testDialector{connPool: userConn}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open gorm DB: %v", err)
	}

	// Simulate a cancelled context, tx.Error != nil, and stale Statement.Vars before deferred AbortBatch runs.
	ctx, cancel := context.WithCancel(context.Background())
	wantErr := errors.New("simulated failure")
	_, err = RunDDLBatch(gormDB.WithContext(ctx), false, func(tx *gorm.DB) error {
		assertInDDLBatch(t, userConn, true)
		tx.Statement.Vars = append(tx.Statement.Vars, "stale-var")
		_ = tx.AddError(wantErr)
		cancel()
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunDDLBatch error mismatch\nGot:  %v\nWant: %v", err, wantErr)
	}

	// Verify AbortBatch actually executed on the persistent userConn despite cancelled ctx and tx.Error != nil.
	assertInDDLBatch(t, userConn, false)
}

func TestRunDDLBatch_DryRunAndCustomConnPool(t *testing.T) {
	t.Parallel()

	pool := &mockConnPool{}
	gormDB, err := gorm.Open(testDialector{connPool: nil}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open gorm DB: %v", err)
	}
	// Custom pool set only on Statement.ConnPool.
	gormDB.Statement.ConnPool = pool
	if _, err := RunDDLBatch(gormDB, false, func(tx *gorm.DB) error {
		return nil
	}); err != nil {
		t.Fatalf("RunDDLBatch with custom Statement.ConnPool failed: %v", err)
	}
	if g, w := pool.execQueries, []string{"START BATCH DDL", "RUN BATCH"}; !slices.Equal(g, w) {
		t.Fatalf("custom pool query mismatch\nGot:  %v\nWant: %v", g, w)
	}

	// GORM DryRun mode should skip connection checkout and execute no driver SQL.
	dryDB := gormDB.Session(&gorm.Session{DryRun: true})
	if _, err := RunDDLBatch(dryDB, false, func(tx *gorm.DB) error {
		return nil
	}); err != nil {
		t.Fatalf("RunDDLBatch in DryRun mode failed: %v", err)
	}
	if g, w := len(pool.execQueries), 2; g != w {
		t.Errorf("DryRun mode should not execute SQL on pool\nGot:  %v\nWant: %v", g, w)
	}

	// Combining GORM DryRun with Spanner AutoMigrateDryRun (dryRun=true) returns an error even on a real *sql.Conn.
	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()
	userConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to check out sql.Conn: %v", err)
	}
	defer func() { _ = userConn.Close() }()

	dryConnDB, err := gorm.Open(testDialector{connPool: userConn}, &gorm.Config{
		DryRun: true,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open gorm DB: %v", err)
	}
	if _, err := RunDDLBatch(dryConnDB, true, func(tx *gorm.DB) error {
		return nil
	}); err == nil {
		t.Errorf("expected error when calling RunDDLBatch dryRun=true in GORM DryRun mode, got nil")
	}
}

func TestGetBatchedStatements_ClosedConn(t *testing.T) {
	t.Parallel()

	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()

	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to check out sql.Conn: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("failed to close sql.Conn: %v", err)
	}

	db := &gorm.DB{Config: &gorm.Config{ConnPool: conn}}
	if _, err := getBatchedStatements(db); !errors.Is(err, sql.ErrConnDone) {
		t.Errorf("getBatchedStatements error mismatch\nGot:  %v\nWant: %v", err, sql.ErrConnDone)
	}
}

func TestRunDDLBatch_DryRun(t *testing.T) {
	t.Parallel()

	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()

	userConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to check out sql.Conn: %v", err)
	}
	defer func() { _ = userConn.Close() }()

	gormDB, err := gorm.Open(testDialector{connPool: userConn}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open gorm DB: %v", err)
	}

	statements, err := RunDDLBatch(gormDB, true, func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE t (id INT64) PRIMARY KEY (id)").Error
	})
	if err != nil {
		t.Fatalf("RunDDLBatch dry-run failed: %v", err)
	}
	if g, w := len(statements), 1; g != w {
		t.Fatalf("batched statement count mismatch\nGot:  %v\nWant: %v", g, w)
	}
	assertInDDLBatch(t, userConn, false)
}

func TestRunDDLBatch_StartBatchDDLFailure(t *testing.T) {
	t.Parallel()

	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()

	userConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to check out sql.Conn: %v", err)
	}
	defer func() { _ = userConn.Close() }()

	gormDB, err := gorm.Open(testDialector{connPool: userConn}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open gorm DB: %v", err)
	}

	if err := StartBatchDDL(gormDB); err != nil {
		t.Fatalf("initial StartBatchDDL failed: %v", err)
	}
	defer func() { _ = AbortBatch(gormDB) }()

	// Starting a nested DDL batch via RunDDLBatch on the same connection must fail
	// without aborting the caller's outer batch.
	if _, err := RunDDLBatch(gormDB, false, func(tx *gorm.DB) error {
		return nil
	}); err == nil {
		t.Fatal("expected error when starting nested DDL batch, got nil")
	}
	assertInDDLBatch(t, userConn, true)
}

func TestRunDDLBatch_TxErrorWithoutReturnedError(t *testing.T) {
	t.Parallel()

	sqlDB, cleanup := setupMockDB(t)
	defer cleanup()

	userConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to check out sql.Conn: %v", err)
	}
	defer func() { _ = userConn.Close() }()

	gormDB, err := gorm.Open(testDialector{connPool: userConn}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open gorm DB: %v", err)
	}

	wantErr := errors.New("tx error")
	_, err = RunDDLBatch(gormDB, false, func(tx *gorm.DB) error {
		_ = tx.AddError(wantErr)
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunDDLBatch error mismatch\nGot:  %v\nWant: %v", err, wantErr)
	}
	assertInDDLBatch(t, userConn, false)
}
