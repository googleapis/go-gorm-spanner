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
	"fmt"

	spannerpg "github.com/googleapis/go-gorm-spanner/postgresql"
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// HelloWorld is a simple sample application that shows how to connect gorm to Spanner PostgreSQL.
//
// Execute the sample with the command `go run run_sample.go hello_world` from this directory.
func HelloWorld(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannerpg.New(postgres.Config{
		DSN: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}
	var msg string

	// Execute a SQL statement that uses the PostgreSQL dialect through gorm.
	if err := db.Raw("SELECT $1::varchar as greeting", "Hello World from Spanner PostgreSQL").Scan(&msg).Error; err != nil {
		return fmt.Errorf("failed to execute query: %v", err)
	}
	fmt.Println(msg)

	return nil
}
