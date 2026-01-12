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

package snippets

import (
	"database/sql"
	"fmt"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	spannerpg "github.com/googleapis/go-gorm-spanner/postgresql"
	"github.com/googleapis/go-gorm-spanner/postgresql/samples/snippets/sample_model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// AutoSaveAssociations shows how to create a model with one or more associated
// models in one Create call. gorm uses an insert-or-update statement for these
// calls.
//
// Execute the sample with the command `go run run_sample.go auto_save_associations`
// from the samples directory.
func AutoSaveAssociations(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannerpg.New(postgres.Config{
		DSN: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Insert a singer and a few albums.
	// gorm allows us to create these in one go by creating the model hierarchy
	// directly in code, and then submitting the top-level model to the Create
	// function.
	singer := sample_model.Singer{
		FirstName: sql.NullString{String: "Angel", Valid: true},
		LastName:  "Woodward",
		Active:    true,
		Albums: []sample_model.Album{
			{
				Title:       "Fine Stuff",
				ReleaseDate: spanner.NullDate{Date: civil.Date{Year: 2024, Month: 11, Day: 11}, Valid: true},
			},
			{
				Title:       "Better Things",
				ReleaseDate: spanner.NullDate{Date: civil.Date{Year: 2023, Month: 1, Day: 30}, Valid: true},
			},
			{
				Title:       "All Good",
				ReleaseDate: spanner.NullDate{Date: civil.Date{Year: 2022, Month: 5, Day: 5}, Valid: true},
			},
		},
	}
	db = db.Create(&singer)
	if db.Error != nil {
		return db.Error
	}

	// Note that gorm only returns the number of affected rows for the top-level
	// record, i.e. the number of singers that were inserted.
	fmt.Printf("Inserted %d singer\n", db.RowsAffected)

	// By loading the singer from the database again, we can see that the albums
	// were also added to the database.
	db = db.Debug().Preload("Albums").Find(&singer)
	if db.Error != nil {
		return db.Error
	}
	fmt.Printf("Singer %s has %d albums\n", singer.FullName, len(singer.Albums))

	return nil
}
