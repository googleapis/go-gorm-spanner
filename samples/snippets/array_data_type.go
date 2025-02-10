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

	"cloud.google.com/go/spanner"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/gorm"
)

type TicketSaleWithNullArray struct {
	gorm.Model
	CustomerName string
	Seats        spannergorm.NullStringArray
	Concert      *sample_model.Concert
	ConcertId    int64
}

func (TicketSaleWithNullArray) TableName() string {
	return "ticket_sales"
}

type TicketSaleWithNativeArray struct {
	gorm.Model
	CustomerName string
	Seats        spannergorm.StringArray
	Concert      *sample_model.Concert
	ConcertId    int64
}

func (TicketSaleWithNativeArray) TableName() string {
	return "ticket_sales"
}

// ArrayDataType shows how to map ARRAY columns in Spanner when using gorm.
//
// Execute the sample with the command `go run run_sample.go array_data_type` from this directory.
func ArrayDataType(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	concert, err := createSingerVenueAndConcert(db)
	if err != nil {
		return err
	}
	// Create a TicketSale struct that uses a NullString array.
	ticketSale := TicketSaleWithNullArray{
		Concert:      &concert,
		CustomerName: "Christin Chukwuma",
		Seats: []spanner.NullString{
			{Valid: true, StringVal: "A10"},
			/* This is a NULL */ {},
			{Valid: true, StringVal: "A11"},
		},
	}
	if err := db.Create(&ticketSale).Error; err != nil {
		return err
	}
	fmt.Printf("Created ticket sale #%v\n", ticketSale.ID)

	// Read the ticket sale back from the database.
	var ts TicketSaleWithNullArray
	if err := db.Find(&ts, ticketSale.ID).Error; err != nil {
		return err
	}
	fmt.Printf("Found ticket sale %v with seats %v\n", ticketSale.ID, ticketSale.Seats)

	// Create a database connection that by default decodes arrays to native Go arrays.
	// This option should only be used if the database only contains arrays without any
	// null elements in the arrays.
	db, err = gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s?decodeToNativeArrays=true", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	// Use the native array type. This type does not support NULL elements in the array.
	// Create a TicketSale struct that uses a string array.
	ticketSale2 := TicketSaleWithNativeArray{
		Concert:      &concert,
		CustomerName: "Emma Brown",
		Seats:        []string{"A12", "A13"},
	}
	if err := db.Create(&ticketSale2).Error; err != nil {
		return err
	}
	fmt.Printf("Created ticket sale #%v\n", ticketSale2.ID)

	// Read the ticket sale back from the database.
	var ts2 TicketSaleWithNativeArray
	if err := db.Find(&ts2, ticketSale2.ID).Error; err != nil {
		return err
	}
	fmt.Printf("Found ticket sale %v with seats %v\n", ticketSale2.ID, ticketSale2.Seats)

	return nil
}

func createSingerVenueAndConcert(db *gorm.DB) (sample_model.Concert, error) {
	singer := sample_model.Singer{
		FirstName: sql.NullString{String: "Angel", Valid: true},
		LastName:  "Woodward",
		Active:    true,
	}
	if err := db.Create(&singer).Error; err != nil {
		return sample_model.Concert{}, err
	}
	venue := sample_model.Venue{
		Name: "Concert Hall",
	}
	if err := db.Create(&venue).Error; err != nil {
		return sample_model.Concert{}, err
	}
	concert := sample_model.Concert{
		Name:   "Woodward in Concert Hall",
		Venue:  venue,
		Singer: singer,
	}
	if err := db.Create(&concert).Error; err != nil {
		return sample_model.Concert{}, err
	}
	return concert, nil
}
