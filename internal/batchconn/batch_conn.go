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
	"errors"
	"fmt"

	"cloud.google.com/go/spanner"
	spannerdriver "github.com/googleapis/go-sql-spanner"
	"gorm.io/gorm"
)

var errDedicatedConnRequired = errors.New("DDL batching requires a dedicated connection; wrap calls in db.Connection(...) or configure Dialector with a *sql.Conn")

// RunDDLBatch executes fn inside a DDL batch on a dedicated *sql.Conn,
// returning the connection to the pool when done. When dryRun is true, it
// returns the batched statements and aborts the batch instead of executing it.
func RunDDLBatch(db *gorm.DB, dryRun bool, fn func(tx *gorm.DB) error) ([]spanner.Statement, error) {
	if err := validateDB(db); err != nil {
		return nil, err
	}
	if db.DryRun && dryRun {
		return nil, errors.New("AutoMigrateDryRun cannot be used with gorm DryRun mode")
	}
	var statements []spanner.Statement
	if err := runInConnection(db, func(tx *gorm.DB) error {
		if err := StartBatchDDL(tx); err != nil {
			return err
		}
		batchFinished := false
		defer func() {
			if !batchFinished {
				_ = AbortBatch(tx)
			}
		}()

		if err := fn(tx); err != nil {
			return err
		}
		if tx.Error != nil {
			return tx.Error
		}
		if dryRun {
			var err error
			statements, err = getBatchedStatements(tx)
			if err != nil {
				return err
			}
			batchFinished = true
			return AbortBatch(tx)
		}
		if err := RunBatch(tx); err != nil {
			return err
		}
		batchFinished = true
		return nil
	}); err != nil {
		return nil, err
	}
	return statements, nil
}

func validateDB(db *gorm.DB) error {
	if db == nil || db.Config == nil || db.Statement == nil || (!db.DryRun && !hasCustomConnPool(db)) {
		return gorm.ErrInvalidDB
	}
	return nil
}

func runInConnection(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	if err := validateDB(db); err != nil {
		return err
	}
	if db.DryRun {
		return fn(db)
	}
	if _, ok := underlyingConn(db); ok {
		return fn(db)
	}
	if _, err := db.DB(); errors.Is(err, gorm.ErrInvalidDB) && hasCustomConnPool(db) {
		// ConnPool is a custom non-*sql.DB pool (e.g. a mock); run directly on it.
		return fn(db)
	}
	return db.Connection(fn)
}

// StartBatchDDL starts a DDL batch on db. Calling it on a pooled *sql.DB
// without a pinned *sql.Conn (e.g. via db.Connection or Config{Conn: conn})
// returns an error.
func StartBatchDDL(db *gorm.DB) error {
	if err := validateDB(db); err != nil {
		return err
	}
	if !db.DryRun {
		if _, ok := underlyingConn(db); !ok {
			_, err := db.DB()
			switch {
			case err == nil:
				return errDedicatedConnRequired
			case errors.Is(err, gorm.ErrInvalidDB) && hasCustomConnPool(db):
				// ConnPool is a custom non-*sql.DB pool (e.g. a mock); allow it.
			default:
				return err
			}
		}
	}
	return db.Session(&gorm.Session{NewDB: true}).Exec("START BATCH DDL").Error
}

// RunBatch executes RUN BATCH on db.
func RunBatch(db *gorm.DB) error {
	if err := validateDB(db); err != nil {
		return err
	}
	return db.Session(&gorm.Session{NewDB: true}).Exec("RUN BATCH").Error
}

// AbortBatch executes ABORT BATCH on db, clearing any prior db.Error and
// detaching context cancellation/deadlines so cleanup still executes.
func AbortBatch(db *gorm.DB) error {
	if err := validateDB(db); err != nil {
		return err
	}
	// Pass Context in the NewDB: true session so clone stays 1 and Exec creates
	// a fresh empty Statement without inheriting stale Vars/Clauses from db.
	session := &gorm.Session{NewDB: true}
	if db.Statement.Context != nil {
		session.Context = context.WithoutCancel(db.Statement.Context)
	}
	execDB := db.Session(session)
	execDB.Error = nil
	return execDB.Exec("ABORT BATCH").Error
}

func hasCustomConnPool(db *gorm.DB) bool {
	return db.ConnPool != nil || (db.Statement != nil && db.Statement.ConnPool != nil)
}

func underlyingConn(db *gorm.DB) (*sql.Conn, bool) {
	if db == nil || db.Config == nil {
		return nil, false
	}
	pool := db.ConnPool
	if db.Statement != nil && db.Statement.ConnPool != nil {
		pool = db.Statement.ConnPool
	}
	for pool != nil {
		switch p := pool.(type) {
		case *sql.Conn:
			return p, p != nil
		case *gorm.PreparedStmtDB:
			if p == nil {
				pool = nil
			} else {
				pool = p.ConnPool
			}
		default:
			pool = nil
		}
	}
	return nil, false
}

func getBatchedStatements(db *gorm.DB) ([]spanner.Statement, error) {
	conn, ok := underlyingConn(db)
	if !ok {
		return nil, fmt.Errorf("unexpected ConnPool type")
	}
	var statements []spanner.Statement
	if err := conn.Raw(func(driverConn any) error {
		spannerConn, ok := driverConn.(spannerdriver.SpannerConn)
		if !ok {
			return fmt.Errorf("dry-run is only supported for Spanner")
		}
		statements = spannerConn.GetBatchedStatements()
		return nil
	}); err != nil {
		return nil, err
	}
	return statements, nil
}
