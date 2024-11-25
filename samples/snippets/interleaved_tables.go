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

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	"gorm.io/gorm"
)

// InterleavedTables shows how to map and use interleaved tables with gorm.
// Interleaved tables can be mapped as a 'has many' (https://gorm.io/docs/has_many.html)
// association in gorm.
//
// Execute the sample with the command `go run run_sample.go interleaved_tables`
// from the samples directory.
func InterleavedTables(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Insert a singer, an album, and a few tracks belonging to that album.
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
				// Track is INTERLEAVED IN PARENT Album.
				// They share the same primary key prefix (ID).
				// Track adds a TrackNumber column to the primary key.
				// The list of tracks can be used as a normal association in gorm.
				Tracks: []sample_model.Track{
					{TrackNumber: 1, Title: "This is fine"},
					{TrackNumber: 2, Title: "Wish it was better"},
					{TrackNumber: 3, Title: "Probably the best"},
					{TrackNumber: 4, Title: "Things are getting better"},
					{TrackNumber: 5, Title: "Why is this still here?"},
				},
			},
		},
	}
	// Note: gorm by default tries to only update the association columns when you
	// auto-create association. This is not supported by Spanner, as Spanner requires
	// either all columns to be updated, or none (INSERT OR IGNORE).
	// By adding `FullSaveAssociations: true` to the session when using auto-save
	// associations, gorm will generate an INSERT OR UPDATE statement.
	// Failing to add `FullSaveAssociations: true` will lead to the following error:
	// 'spanner only supports UpdateAll or DoNothing for OnConflict clauses'.
	db = db.Session(&gorm.Session{FullSaveAssociations: true}).Create(&singer)
	if db.Error != nil {
		return db.Error
	}
	fmt.Printf("Inserted %d singer\n", db.RowsAffected)

	// Verify that we can iterate over the tracks of an album as if it were a normal association.
	// Read the singer from the database.
	id := singer.ID
	singer = sample_model.Singer{}
	singer.ID = id
	// Load the singer from the database and preload the relevant associations.
	db = db.Preload("Albums").Preload("Albums.Tracks").Find(&singer)
	if db.Error != nil {
		return db.Error
	}
	fmt.Printf("Singer %s has %d albums\n", singer.FullName, len(singer.Albums))
	for _, album := range singer.Albums {
		fmt.Printf("\tAlbum %s has %d tracks\n", album.Title, len(album.Tracks))
		for _, track := range album.Tracks {
			fmt.Printf("\t\t%d: %s\n", track.TrackNumber, track.Title)
		}
	}

	return nil
}
