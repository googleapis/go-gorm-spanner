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
	"strings"

	"github.com/google/uuid"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// FindInBatches shows how to process large amounts of data in smaller batches.
// This reduces the time that locks are held, and ensures that the Spanner
// transaction mutation limit is not exceeded.
//
// Execute the sample with the command `go run run_sample.go find_in_batches` from this directory.
func FindInBatches(projectId, instanceId, databaseId string) error {
	l := logger.Default.LogMode(logger.Info)
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{Logger: l})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Insert 50 test singer records.
	if err := insertTestSingers(db); err != nil {
		return err
	}

	// Fetch and process singers in batches of 10. This ensures that locks are not held for longer
	// than necessary, and that the Spanner transaction mutation limit is not exceeded.
	// See https://cloud.google.com/spanner/quotas#limits-for for more information on Spanner limits.

	var singers []*sample_model.Singer
	db.FindInBatches(&singers, 10, func(tx *gorm.DB, batch int) error {
		for _, singer := range singers {
			if strings.HasSuffix(singer.FirstName.String, "1") {
				singer.Active = false
				if err := tx.Save(singer).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})

	return nil
}

func insertTestSingers(db *gorm.DB) error {
	// Insert a batch of 50 singers.
	numSingers := 50
	singers := make([]sample_model.Singer, 0, numSingers)
	for i := 0; i < numSingers; i++ {
		singers = append(singers, sample_model.Singer{
			FirstName: sql.NullString{String: fmt.Sprintf("First %d", i), Valid: true},
			LastName:  uuid.New().String(),
			Active:    true,
		})
	}
	return db.CreateInBatches(&singers, 500).Error
}
