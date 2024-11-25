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

	"github.com/google/uuid"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	"gorm.io/gorm"
)

type user struct {
	// gorm automatically assumes that the field with the name ID is the primary
	// key of the table. It also assumes that it is not auto-generated if the type
	// is a string. We limit the length of the column to 36 characters, so it fits
	// a well-formatted UUID string.
	ID   string `gorm:"size:36"`
	Name string
}

// UuidPrimaryKey shows how to use a client-side generated UUID as the
// primary key in gorm. Using a primary key that is either generated
// client-side or otherwise set by the client, means that gorm does not
// need to use a THEN RETURN clause to return the primary key from the
// server.
//
// Execute the sample with the command `go run run_sample.go uuid_primary_key`
// from the samples directory.
func UuidPrimaryKey(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Create the `users` table. This table does not have an auto-generated primary key.
	if err := db.Migrator().AutoMigrate(&user{}); err != nil {
		return fmt.Errorf("could not execute migrations: %v", err)
	}
	// Insert some user records in the database.
	if err := db.CreateInBatches([]*user{
		{ID: uuid.NewString(), Name: "User 1"},
		{ID: uuid.NewString(), Name: "User 2"},
		{ID: uuid.NewString(), Name: "User 3"},
		{ID: uuid.NewString(), Name: "User 4"},
	}, 100).Error; err != nil {
		return err
	}

	// Verify that the records were created.
	var users []user
	if err := db.Model(&user{}).Order("name").Find(&users).Error; err != nil {
		return err
	}
	fmt.Println("Found these users:")
	for _, user := range users {
		fmt.Printf("%s %s\n", user.ID, user.Name)
	}

	return nil
}
