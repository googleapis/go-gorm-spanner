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
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/gorm"
)

// BatchDml shows how to use the START BATCH DML / RUN BATCH feature in Spanner to buffer
// multiple update statements and execute these in one round-trip to Spanner.
//
// Execute the sample with the command `go run run_sample.go batch_dml` from this directory.
func BatchDml(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Insert 50 test singer records.
	if err := insertTestSingers(db); err != nil {
		return err
	}

	// Run a read/write transaction on Spanner that fetches all singers, and updates the Active
	// flag of each singer as a separate statement.
	// The START BATCH DML / RUN BATCH statements ensure that these single statements are sent
	// to Spanner as a single batch. This happens in the following way:
	// 1. START BATCH DML creates a DML batch on the transaction.
	// 2. All DML statements on the same transaction are now buffered in memory instead of being sent
	//    directly to Spanner.
	// 3. RUN BATCH flushes all statements that have been buffered.
	var singers []*sample_model.Singer
	return spannergorm.RunTransaction(context.Background(), db, func(tx *gorm.DB) error {
		if err := tx.Order("last_name").Find(&singers).Error; err != nil {
			return err
		}
		// Start a DML batch. This will buffer all DML statements in memory until
		// RUN BATCH is executed.
		if err := tx.Exec("START BATCH DML").Error; err != nil {
			return err
		}
		for _, singer := range singers {
			singer.Active = false
			if err := tx.Save(singer).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
		// Execute RUN BATCH. This flushes all buffered DML statements to Spanner.
		if err := tx.Exec("RUN BATCH").Error; err != nil {
			return err
		}
		fmt.Println("Executed 50 updates in a single DML batch on Spanner")
		return nil
	})
}
