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
	"math/rand"

	"cloud.google.com/go/spanner"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	"gorm.io/gorm"
)

// ReadWriteTransaction shows how to use read/write transactions with Spanner and gorm.
// Read/write transactions guarantee the consistency and atomicity of multiple queries
// and updates on Spanner. Read/write transactions take locks on the rows that are read
// and updated, and should therefore be as short as possible, and only read/update the
// data that it actually needs.
//
// Read/write transactions on Spanner take locks at the cell level (row/column). It is
// therefore recommended to only read the columns that you actually need.
//
// Spanner can abort any read/write transaction due to lock conflicts or due to transient
// failures (e.g. network errors, machine restarts, etc.). Transactions that fail with
// an Aborted error should be retried. The Spanner gorm dialect provides the helper
// function `spannergorm.RunTransaction` for this.
//
// Execute the sample with the command `go run run_sample.go read_write_transaction`
// from the samples directory.
func ReadWriteTransaction(projectId, instanceId, databaseId string) error {
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

	// Execute a read/write transaction. Spanner takes locks for all data that is
	// read or updated during a read/write transaction.
	// RunTransaction automatically retries the transaction if it
	// is aborted by Spanner. It is recommended to use this helper function for
	// all read/write transactions.
	if err := spannergorm.RunTransaction(context.Background(), db, func(tx *gorm.DB) error {
		// Whenever possible, only select the columns and rows that you actually
		// need in a read/write transaction to prevent taking more locks than
		// necessary.
		var venues []*sample_model.Venue
		if err := tx.Select("id", "venue_details").Where("venue_details is null").Find(&venues).Error; err != nil {
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
				return err
			}
			fmt.Printf("Updated the details of the Venue with id %d\n", venue.ID)
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}
