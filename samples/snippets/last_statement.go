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

	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/gorm"
)

// LastStatement shows how you can disable the use of default transactions in gorm to use
// the last_statement optimization in Spanner.
// This sample inserts a batch of data into Spanner in one round-trip using gorm while
// skipping a default transaction. This again allows the underlying database/sql driver
// to set the last_statement=true flag, which again allows Spanner to optimize the
// execution of that statement.
//
// See https://cloud.google.com/spanner/docs/reference/rpc/google.spanner.v1#executesqlrequest
// for more information on the last_statement option.
//
// Execute the sample with the command `go run run_sample.go last_statement` from this directory.
func LastStatement(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Create a slice of Singers and insert these in one batch without a default transaction.
	singers := []sample_model.Singer{
		{FirstName: sql.NullString{String: "Angel", Valid: true}, LastName: "Woodward", Active: true},
		{FirstName: sql.NullString{String: "Briley", Valid: true}, LastName: "Guerrero", Active: true},
		{FirstName: sql.NullString{String: "Angie", Valid: true}, LastName: "Barker", Active: true},
		{FirstName: sql.NullString{String: "Holden", Valid: true}, LastName: "Duke", Active: true},
		{FirstName: sql.NullString{String: "Hugo", Valid: true}, LastName: "Faulkner", Active: true},
	}
	// Create a gorm session that skips default transactions and execute the insert statement.
	// This instructs gorm to use 'auto-commit' for the statement, which again tells the underlying
	// database/sql driver that it is safe to mark the statement as the last statement in the
	// transaction. This allows Spanner to optimize the execution of the statement and defer some
	// validations until the Commit call.
	db = db.Session(&gorm.Session{SkipDefaultTransaction: true}).Create(&singers)
	if db.Error != nil {
		return db.Error
	}
	fmt.Printf("Inserted %d singers without a default transaction\n", db.RowsAffected)

	// The primary key is auto-generated for the singers. This is returned by Spanner after the insert.
	for _, singer := range singers {
		if singer.ID == 0 {
			return fmt.Errorf("singer %s %s was not assigned a primary key", singer.FirstName.String, singer.LastName)
		}
	}

	return nil
}
