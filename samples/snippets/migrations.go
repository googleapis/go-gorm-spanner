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
	"fmt"

	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Migrations shows how to (dry-)run gorm migrations with Spanner.
// Not all Spanner features can be created with gorm migrations.
// E.g. INTERLEAVED tables are not supported by gorm, and can not
// be created automatically using migrations.
//
// It is therefore recommended to dry-run migrations first and
// inspect the DDL statements that are generated. Modify and
// execute these manually if you want to create interleaved tables,
// or if the generated data model for example contains more secondary
// indexes than you actually want in your database.
//
// Execute the sample with the command `go run run_sample.go migrations`
// from the samples directory.
func Migrations(projectId, instanceId, databaseId string) error {
	l := logger.Default.LogMode(logger.Info)
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true, Logger: l})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	tables := []interface{}{
		&sample_model.Singer{},
		&sample_model.Album{},
		&sample_model.Track{},
		&sample_model.Venue{},
		&sample_model.Concert{},
	}

	// Unwrap the underlying SpannerMigrator interface. This interface supports
	// the `AutoMigrateDryRun` method, which does not actually execute the
	// generated statements, and instead just returns these as an array.
	m := db.Migrator()
	migrator, ok := m.(spannergorm.SpannerMigrator)
	if !ok {
		return fmt.Errorf("unexpected migrator type: %v", m)
	}
	// Dry-run the migrations and print the generated statements.
	statements, err := migrator.AutoMigrateDryRun(tables...)
	if err != nil {
		return fmt.Errorf("could not dry-run migrations: %v", err)
	}
	fmt.Print("\nMigrations dry-run generated these statements:\n\n")
	for _, statement := range statements {
		fmt.Printf("%s;\n", statement.SQL)
	}

	// Run the same migration for real if you are content with the
	// outcome of the dry run.
	if err := migrator.AutoMigrate(tables...); err != nil {
		return fmt.Errorf("could not execute migrations: %v", err)
	}
	fmt.Println("Executed migrations on Spanner")

	return nil
}
