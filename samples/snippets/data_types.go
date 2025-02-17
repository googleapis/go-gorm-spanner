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
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/gorm"
)

// AllTypes shows the supported native Go type mappings to Spanner.
// Most of these types do not support NULL values.
type AllTypes struct {
	ID           int64
	ColBool      bool
	ColBytes     []byte
	ColDate      civil.Date `gorm:"type:date"`
	ColFloat32   float32
	ColFloat64   float64
	ColInt64     int64
	ColJson      spanner.NullJSON
	ColNumeric   big.Rat `gorm:"type:numeric"`
	ColString    string
	ColTimestamp time.Time
}

// SqlNullTypes shows the sql.Null* types that can be used with Spanner.
type SqlNullTypes struct {
	ID           int64
	ColBool      sql.NullBool
	ColFloat64   sql.NullFloat64
	ColInt64     sql.NullInt64
	ColString    sql.NullString
	ColTimestamp sql.NullTime
}

// AllNullTypes shows how the Spanner-specific Null* types can be used to
// map any data type in Spanner.
type AllNullTypes struct {
	ID           int64
	ColBool      spanner.NullBool
	ColDate      spanner.NullDate
	ColFloat32   spanner.NullFloat32
	ColFloat64   spanner.NullFloat64
	ColInt64     spanner.NullInt64
	ColJson      spanner.NullJSON
	ColNumeric   spanner.NullNumeric
	ColString    spanner.NullString
	ColTimestamp spanner.NullTime
}

// DataTypes shows the supported data type mappings for Spanner when using gorm.
// This sample only contains samples for 'simple' data types. See the following
// samples for more complex data types:
// 1. Arrays: array_data_type.go
// 2. Protobuf: protobuf_columns.go
//
// Execute the sample with the command `go run run_sample.go data_types` from this directory.
func DataTypes(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// The listed data types support AutoMigrate.
	// civil.Date and big.Rat require an annotation on the field for gorm to be able to
	// determine the correct data type that should be used when creating the column.
	if err := db.AutoMigrate(&AllTypes{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&SqlNullTypes{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&AllNullTypes{}); err != nil {
		return err
	}

	fmt.Println("Created tables with all supported data types")

	// Create a test row for each mapped struct.
	timeValue, err := time.Parse(time.RFC3339, "2025-02-17T11:14:00+01:00")
	if err != nil {
		return err
	}
	rowAllTypes := AllTypes{
		ID:           1,
		ColBool:      true,
		ColBytes:     []byte("test"),
		ColDate:      civil.Date{Year: 2025, Month: 2, Day: 17},
		ColFloat32:   float32(3.14),
		ColFloat64:   3.14,
		ColInt64:     int64(-1),
		ColJson:      spanner.NullJSON{Value: "{\"key\": \"value\"}", Valid: true},
		ColNumeric:   *(&big.Rat{}).SetFloat64(3.14),
		ColString:    "test",
		ColTimestamp: timeValue,
	}
	if err := db.Create(&rowAllTypes).Error; err != nil {
		return err
	}

	rowSqlNullTypes := SqlNullTypes{
		ID:           1,
		ColBool:      sql.NullBool{Bool: rowAllTypes.ColBool, Valid: true},
		ColFloat64:   sql.NullFloat64{Float64: rowAllTypes.ColFloat64, Valid: true},
		ColInt64:     sql.NullInt64{Int64: rowAllTypes.ColInt64, Valid: true},
		ColString:    sql.NullString{String: rowAllTypes.ColString, Valid: true},
		ColTimestamp: sql.NullTime{Time: rowAllTypes.ColTimestamp, Valid: true},
	}
	if err := db.Create(&rowSqlNullTypes).Error; err != nil {
		return err
	}

	rowAllNullTypes := AllNullTypes{
		ID:           1,
		ColBool:      spanner.NullBool{Bool: rowAllTypes.ColBool, Valid: true},
		ColDate:      spanner.NullDate{Date: rowAllTypes.ColDate, Valid: true},
		ColFloat32:   spanner.NullFloat32{Float32: rowAllTypes.ColFloat32, Valid: true},
		ColFloat64:   spanner.NullFloat64{Float64: rowAllTypes.ColFloat64, Valid: true},
		ColInt64:     spanner.NullInt64{Int64: rowAllTypes.ColInt64, Valid: true},
		ColJson:      spanner.NullJSON{Value: rowAllTypes.ColJson.Value, Valid: true},
		ColNumeric:   spanner.NullNumeric{Numeric: rowAllTypes.ColNumeric, Valid: true},
		ColString:    spanner.NullString{StringVal: rowAllTypes.ColString, Valid: true},
		ColTimestamp: spanner.NullTime{Time: rowAllTypes.ColTimestamp, Valid: true},
	}
	if err := db.Create(&rowAllNullTypes).Error; err != nil {
		return err
	}

	fmt.Println("Inserted test rows with all supported data types")

	return nil
}
