// Copyright 2025 Google LLC.
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

package snippets

import (
	"database/sql"
	"fmt"
	"math/rand"

	"cloud.google.com/go/spanner"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	"gorm.io/gorm"
)

// IsolationLevel shows how to use a custom transaction isolation level with Spanner and
// gorm.
//
// Read/write transactions on Spanner take locks at the cell level (row/column). It is
// therefore recommended to only read the columns that you actually need.
//
// Spanner can abort any read/write transaction due to lock conflicts or due to transient
// failures (e.g. network errors, machine restarts, etc.). Transactions that fail with
// an Aborted error should be retried. The Spanner gorm dialect provides the helper
// function `spannergorm.RunTransaction` for this.
//
// Execute the sample with the command `go run run_sample.go isolation_level`
// from the samples directory.
func IsolationLevel(projectId, instanceId, databaseId string) error {
	// You can set the default isolation level in the connection string.
	// All read/write transactions on this database connection will use this
	// isolation level, unless an isolation level is set in the transaction
	// options of a specific transaction.
	// Spanner supports the following isolation levels:
	// 1. sql.LevelSerializable
	// 2. sql.LevelRepeatableRead
	//
	// The default is sql.LevelSerializable. This sample changes that default
	// to sql.LevelRepeatableRead.
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN: fmt.Sprintf("projects/%s/instances/%s/databases/%s;isolationLevel=%s",
			projectId, instanceId, databaseId, sql.LevelRepeatableRead),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Insert a test row.
	if err := insertVenue(db); err != nil {
		return err
	}

	// Execute a read/write transaction with the default isolation level.
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := executeTx(tx); err != nil {
		return err
	}
	fmt.Printf("Executed transaction with isolation level %s\n", sql.LevelRepeatableRead)

	// Execute a read/write transaction with a specific isolation level.
	tx = db.Begin(&sql.TxOptions{Isolation: sql.LevelSerializable})
	if tx.Error != nil {
		return tx.Error
	}
	if err := executeTx(tx); err != nil {
		return err
	}
	fmt.Printf("Executed transaction with isolation level %s\n", sql.LevelSerializable)

	return nil
}

func executeTx(tx *gorm.DB) error {
	var venues []*sample_model.Venue
	if err := tx.Select("id", "venue_details").Where("venue_details is null").Find(&venues).Error; err != nil {
		tx.Rollback()
		return err
	}
	for _, venue := range venues {
		venue.VenueDetails = spanner.NullJSON{
			Value: sample_model.VenueDetails{
				Rating: spanner.NullFloat64{Float64: rand.Float64() * 10, Valid: true},
				Open:   true,
			},
			Valid: true,
		}
		if err := tx.Model(venue).Update("venue_details", venue.VenueDetails).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	return nil
}
