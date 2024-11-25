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
)

// CreateInBatches shows how to use the corresponding method in gorm to create multiple
// records in a limited number of round-trips to Spanner.
//
// Execute the sample with the command `go run run_sample.go hello_world` from this directory.
func CreateInBatches(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Create a slice of Singers and insert these in batches of 5.
	numSingers := 50
	singers := make([]sample_model.Singer, 0, numSingers)
	for i := 0; i < numSingers; i++ {
		singers = append(singers, sample_model.Singer{
			FirstName: sql.NullString{String: fmt.Sprintf("First"), Valid: true},
			LastName:  fmt.Sprintf("Last %d", i),
			Active:    true,
			Albums: []sample_model.Album{
				{Title: "Album 1"},
				{Title: "Album 2"},
				{Title: "Album 3"},
				{Title: "Album 4"},
			},
		})
	}
	// gorm by default tries to only update the association columns when you
	// auto-create association. This is not supported by Spanner, as Spanner requires
	// either all columns to be updated, or none (INSERT OR IGNORE).
	//
	// By adding `FullSaveAssociations: true` to the session when using auto-save
	// associations, gorm will generate an INSERT OR UPDATE statement.
	//
	// Failing to add `FullSaveAssociations: true` will lead to the following error:
	// 'spanner only supports UpdateAll or DoNothing for OnConflict clauses'.
	db.Session(&gorm.Session{FullSaveAssociations: true}).CreateInBatches(&singers, 5)
	if db.Error != nil {
		return db.Error
	}

	// CreateInBatches does not return the affected row count, so we get the number of inserted
	// singers by executing a query.
	count := 0
	db.Raw("select count(1) from singers").Scan(&count)
	if db.Error != nil {
		return db.Error
	}
	fmt.Printf("Inserted %d singers\n", count)

	return nil
}
