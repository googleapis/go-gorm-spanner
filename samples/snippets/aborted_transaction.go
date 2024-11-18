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

package snippets

import (
	"context"
	"fmt"

	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	"gorm.io/gorm"
)

// AbortedTransaction shows how transaction retries work on Spanner.
// Read/write transactions guarantee the consistency and atomicity of multiple queries
// and updates on Spanner. Read/write transactions take locks on the rows that are read
// and updated. Spanner can abort any read/write transaction due to lock conflicts or
// due to transient failures (e.g. network errors, machine restarts, etc.).
//
// Transactions that fail with an Aborted error should be retried. The Spanner gorm
// dialect provides the helper function `spannergorm.RunTransaction`
// for this. It is recommended to run all read/write transactions using this helper
// function, or add a similar retry function to your own application.
//
// Execute the sample with the command `go run run_sample.go aborted_transaction`
// from the samples directory.
func AbortedTransaction(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Insert a test row.
	if err := insertVenue(db); err != nil {
		return err
	}

	// RunTransaction automatically retries the transaction if it
	// is aborted by Spanner. It is recommended to use this helper function for
	// all read/write transactions.
	attempt := 0
	if err := spannergorm.RunTransaction(context.Background(), db, func(tx *gorm.DB) error {
		attempt++
		fmt.Printf("Executing attempt %d of the first transaction\n", attempt)
		// Select the venue row in this transaction.
		var venue sample_model.Venue
		if err := tx.First(&venue).Error; err != nil {
			return err
		}
		if attempt == 1 {
			// Execute another read/write transaction that reads and updates the same row.
			// This will cause this transaction to be aborted by Spanner.
			if err := readAndUpdateVenueInTransaction(db); err != nil {
				return err
			}
		}
		venue.Name = venue.Name + " - Updated in first transaction"
		if err := tx.Updates(&venue).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	fmt.Printf("First transaction succeeded after %d attempt(s)\n", attempt)

	return nil
}

func readAndUpdateVenueInTransaction(db *gorm.DB) error {
	attempt := 0
	if err := spannergorm.RunTransaction(context.Background(), db, func(tx *gorm.DB) error {
		attempt++
		fmt.Printf("Executing attempt %d of the second transaction\n", attempt)
		var venue sample_model.Venue
		if err := tx.First(&venue).Error; err != nil {
			return err
		}
		venue.Name = venue.Name + " - Updated in second transaction"
		if err := tx.Updates(&venue).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	fmt.Printf("Second transaction succeeded after %d attempt(s)\n", attempt)

	return nil
}
