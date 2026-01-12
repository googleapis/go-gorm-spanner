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

package main

import (
	"fmt"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	spannerpg "github.com/googleapis/go-gorm-spanner/postgresql"
	"github.com/googleapis/go-gorm-spanner/samples/emulator"
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Simple sample application that shows how to use gorm with Spanner PostgreSQL.
//
// Execute the sample with the command `go run main.go` from this directory.
func helloWorld(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannerpg.New(postgres.Config{
		DSN: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}
	var msg string
	if err := db.Raw("SELECT $1::varchar as greeting", "Hello World from Spanner PostgreSQL!").Scan(&msg).Error; err != nil {
		return fmt.Errorf("failed to execute query: %v", err)
	}
	fmt.Println(msg)

	return nil
}

func main() {
	emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_POSTGRESQL, helloWorld)
}
