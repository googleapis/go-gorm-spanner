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
	"context"
	"fmt"
	"math/bits"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	spannerpg "github.com/googleapis/go-gorm-spanner/postgresql"
	"github.com/googleapis/go-gorm-spanner/samples/emulator"
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	Code  string
	Price uint
}

// Simple sample application that shows how to use gorm with Spanner PostgreSQL.
// This sample is a copy of the gorm Quickstart application: https://gorm.io/docs/index.html
//
// Execute the sample with the command `go run main.go` from this directory.
func quickstart(projectId, instanceId, databaseId string) error {
	// Open a gorm connection to Spanner PostgreSQL using the spannerpg.New function with a postgres.Config.
	db, err := gorm.Open(spannerpg.New(postgres.Config{
		DSN: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	ctx := context.Background()

	// Migrate the schema
	if err := db.AutoMigrate(&Product{}); err != nil {
		return fmt.Errorf("failed to AutoMigrate: %v\n", err)
	}

	// Create
	err = gorm.G[Product](db).Create(ctx, &Product{Code: "D42", Price: 100})

	// Read
	// Spanner uses bit-reversed sequences for identifier generation.
	reversed := bits.Reverse64(uint64(2))
	product, err := gorm.G[Product](db).Where("id = ?", reversed).First(ctx) // find product with integer primary key
	products, err := gorm.G[Product](db).Where("code = ?", "D42").Find(ctx)  // find product with code D42
	for _, product := range products {
		fmt.Printf("Product: %+v\n", product)
	}

	// Update - update product's price to 200
	if _, err = gorm.G[Product](db).Where("id = ?", product.ID).Update(ctx, "Price", 200); err != nil {
		return err
	}
	// Update - update multiple fields
	if _, err = gorm.G[Product](db).Where("id = ?", product.ID).Updates(ctx, Product{Code: "D42", Price: 100}); err != nil {
		return err
	}
	// Delete - delete product
	if _, err = gorm.G[Product](db).Where("id = ?", product.ID).Delete(ctx); err != nil {
		return err
	}

	return nil
}

func main() {
	emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_POSTGRESQL, quickstart)
}
