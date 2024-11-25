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

	"cloud.google.com/go/spanner"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	spannerdriver "github.com/googleapis/go-sql-spanner"
	"google.golang.org/api/iterator"
	"gorm.io/gorm"
)

// ClientLibrary shows how to unwrap the underlying client library from a gorm
// database, so the client can be used to use specific Spanner features.
//
// The underlying Spanner client is stateless and independent of the connection
// that was used to get it. This means that we can safely use the client after
// the connection that was used to get it has been closed.
//
// Execute the sample with the command `go run run_sample.go client_library`
// from the samples directory.
func ClientLibrary(projectId, instanceId, databaseId string) error {
	ctx := context.Background()
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Insert some test data that can be used in this example.
	if err := insertSingers(db); err != nil {
		return err
	}

	// Get the underlying database/sql connector.
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	// Get a Spanner-specific connection.
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	var client *spanner.Client
	if err := conn.Raw(func(driverConn any) error {
		spannerConn, ok := driverConn.(spannerdriver.SpannerConn)
		if !ok {
			return fmt.Errorf("gorm is not connected to a Spanner database")
		}
		// Getting the underlying Spanner client from a connection does not in any
		// way associate the Spanner client with the connection. A Spanner client
		client, err = spannerConn.UnderlyingClient()
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	// We can safely use the Spanner client after the connection that was used to
	// get it has been closed, because the Spanner client is independent of that
	// connection. A Spanner client is stateless and not connected to the current
	// gorm transaction.

	// Use the Spanner client to execute a read-only transaction and fetch the singers
	// that were added earlier in this sample.
	roTx := client.ReadOnlyTransaction()
	// Make sure that the read-only transaction is closed when we are done with it.
	// Failing to do so, can lead to resource leaks (e.g. session leaks).
	defer roTx.Close()

	// Note that the Query method only prepares the query for execution, it does not
	// actually execute the query. That happens the first time we call it.Next()
	it := roTx.Query(ctx, spanner.Statement{SQL: "select id, full_name from singers order by last_name"})
	// Also make sure to always Stop row iterators. Failing to do so can cause resource leaks.
	defer it.Stop()
	fmt.Println("Selecting all singers")
	for {
		row, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		var id int64
		var name string
		if err := row.Columns(&id, &name); err != nil {
			return err
		}
		fmt.Printf("%d: %s\n", id, name)
	}

	return nil
}
