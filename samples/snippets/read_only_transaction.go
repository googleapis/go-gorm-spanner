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
	"database/sql"
	"fmt"

	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	"gorm.io/gorm"
)

// ReadOnlyTransaction shows how to use read-only transactions with Spanner and gorm.
// Read-only transactions do not take locks, and are therefore more efficient than
// read/write transactions for workloads that only read data.
//
// Execute the sample with the command `go run run_sample.go read_only_transaction`
// from the samples directory.
func ReadOnlyTransaction(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Insert some test data that can be used in a read-only transaction.
	if err := insertSingers(db); err != nil {
		return err
	}

	// Execute a read-only transaction.
	// Read-only transactions do not take locks and therefore do not conflict
	// with any other transactions.
	if err := db.Transaction(func(tx *gorm.DB) error {
		// The read-only transaction sees the data that was inserted before the
		// transaction started.
		var singers []*sample_model.Singer
		if err := tx.Order("last_name").Find(&singers).Error; err != nil {
			return err
		}
		fmt.Printf("Found %d singers\n", len(singers))

		// Insert a venue in a separate transaction.
		if err := insertVenue(db); err != nil {
			return err
		}
		// The venue cannot be read in the current read-only transaction,
		// as it was inserted after this transaction started.
		var venues []*sample_model.Venue
		if err := tx.Find(&venues).Error; err != nil {
			return err
		}
		fmt.Printf("Found %d venues\n", len(venues))

		// Update one of the singers in a separate read/write transaction.
		// This does not create a conflict with the current read-only transaction,
		// as a read-only transaction does not take locks.
		if err := updateSinger(db, singers[0]); err != nil {
			return err
		}
		// The read-only transaction sees a consistent snapshot of the database.
		// Re-selecting the same singer record that was updated above therefore
		// returns the same value as during the initial select at the start of
		// this read-only transaction.
		singer := sample_model.Singer{Model: gorm.Model{ID: singers[0].ID}}
		if err := tx.Find(&singer).Error; err != nil {
			return err
		}
		fmt.Printf("Name of singer:\n1st query: %s\n2nd query: %s\n", singers[0].FullName, singer.FullName)

		return nil
	}, &sql.TxOptions{ReadOnly: true}); err != nil {
		return err
	}

	// Reading the singer after the read-only transaction will fetch the updated value.
	var singer sample_model.Singer
	if err := db.Order("last_name").First(&singer).Error; err != nil {
		return err
	}
	fmt.Printf("Name of singer after read-only transaction: %s\n", singer.FullName)

	return nil
}

func insertSingers(db *gorm.DB) error {
	singers := []sample_model.Singer{
		{FirstName: sql.NullString{String: "Angel", Valid: true}, LastName: "Woodward", Active: true},
		{FirstName: sql.NullString{String: "Briley", Valid: true}, LastName: "Guerrero", Active: true},
		{FirstName: sql.NullString{String: "Angie", Valid: true}, LastName: "Barker", Active: true},
		{FirstName: sql.NullString{String: "Holden", Valid: true}, LastName: "Duke", Active: true},
		{FirstName: sql.NullString{String: "Hugo", Valid: true}, LastName: "Faulkner", Active: true},
	}
	return db.Create(&singers).Error
}

func insertVenue(db *gorm.DB) error {
	return db.Create(&sample_model.Venue{Name: "Test venue"}).Error
}

func updateSinger(db *gorm.DB, singer *sample_model.Singer) error {
	singer.LastName = singer.LastName + " - Updated"
	return db.Save(singer).Error
}
