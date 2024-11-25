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
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Upsert shows how to use INSERT-OR-UPDATE and INSERT-OR-IGNORE with Spanner and gorm.
//
// Execute the sample with the command `go run run_sample.go upsert` from this directory.
func Upsert(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Create a slice of Singers and insert-or-update these in one batch.
	singers := []sample_model.Singer{
		{FirstName: sql.NullString{String: "Angel", Valid: true}, LastName: "Woodward", Active: true},
		{FirstName: sql.NullString{String: "Briley", Valid: true}, LastName: "Guerrero", Active: true},
		{FirstName: sql.NullString{String: "Angie", Valid: true}, LastName: "Barker", Active: true},
		{FirstName: sql.NullString{String: "Holden", Valid: true}, LastName: "Duke", Active: true},
		{FirstName: sql.NullString{String: "Hugo", Valid: true}, LastName: "Faulkner", Active: true},
	}
	// Use the OnConflict(UpdateAll: true) clause to insert-or-update rows.
	// Spanner does not support updating individual columns in an insert-or-update statement.
	db = db.Clauses(clause.OnConflict{UpdateAll: true}).Save(singers)
	if db.Error != nil {
		return db.Error
	}
	fmt.Printf("Upserted %d singers\n", db.RowsAffected)

	// The primary key is auto-generated for the singers. This is returned by Spanner after the insert.
	for _, singer := range singers {
		if singer.ID == 0 {
			return fmt.Errorf("singer %s %s was not assigned a primary key", singer.FirstName.String, singer.LastName)
		}
	}
	// Set the Active flag to false for all singers.
	for _, singer := range singers {
		singer.Active = false
	}
	// Execute an INSERT-OR-IGNORE statement for the singers + one additional singer.
	singers = append(singers, sample_model.Singer{
		FirstName: sql.NullString{String: "Eddie", Valid: true},
		LastName:  "Threepwood",
		Active:    false})
	// Use a OnConflict{DoNothing: true} clause to execute INSERT-OR-IGNORE.
	db = db.Clauses(clause.OnConflict{DoNothing: true}).Save(singers)
	if db.Error != nil {
		return db.Error
	}
	fmt.Printf("Inserted-or-ignored %d singers\n", db.RowsAffected)

	// Read the singers from the database and print them to stdout.
	var list []*sample_model.Singer
	db = db.Find(&list)
	if db.Error != nil {
		return db.Error
	}
	for _, singer := range list {
		fmt.Printf("Singer: %s (Active: %v)\n", singer.FullName, singer.Active)
	}

	return nil
}
