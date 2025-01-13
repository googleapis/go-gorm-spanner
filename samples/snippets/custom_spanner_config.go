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
	"errors"
	"fmt"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	_ "github.com/googleapis/go-sql-spanner"
	spannerdriver "github.com/googleapis/go-sql-spanner"
	"google.golang.org/api/option"
	"gorm.io/gorm"
)

// CustomSpannerConfig shows how to connect GORM to Spanner with a custom configuration
// for the Spanner client.
//
// Execute the sample with the command `go run run_sample.go custom_spanner_config` from this directory.
func CustomSpannerConfig(projectId, instanceId, databaseId string) error {
	dsn := fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId)

	// Create a function that sets the Spanner client configuration for the database connection.
	configureFunction := func(config *spanner.ClientConfig, opts *[]option.ClientOption) {
		// Set a default query optimizer version that the client should use.
		config.QueryOptions = spanner.QueryOptions{Options: &spannerpb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1"}}
	}
	// Create a Connector for Spanner. This Connector instance should be re-used for all gorm connections.
	c, err := spannerdriver.CreateConnector(dsn, configureFunction)

	db, err := gorm.Open(
		spannergorm.New(spannergorm.Config{Connector: c}),
		&gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}
	var msg []string
	if err := db.Raw("SELECT 'Hello World!'").Scan(&msg).Error; err != nil {
		return fmt.Errorf("failed to execute query: %v", err)
	}

	if len(msg) == 0 {
		return errors.New("failed to execute query")
	}
	for _, m := range msg {
		fmt.Printf("%s\n", m)
	}

	return nil
}
