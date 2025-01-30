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
	"database/sql"
	"fmt"
	"time"

	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model"
	"github.com/googleapis/go-gorm-spanner/samples/snippets/sample_model/protos/concertspb"
	"gorm.io/gorm"
)

// ProtobufColumns shows how to map and use protobuf columns with gorm.
//
// Execute the sample with the command `go run run_sample.go protobuf_columns`
// from the samples directory.
func ProtobufColumns(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}

	concert, err := createSingerVenueAndConcert(db)
	if err != nil {
		return err
	}
	// Create a TicketSale object with a protobuf value.
	// Note that concertspb.TicketOrder (the protobuf struct) implements the
	// driver.Valuer and sql.Scanner interfaces.
	// See the sample_model/protos/concertspb folder for the implementation.
	ticketSale := sample_model.TicketSale{
		Concert:      concert,
		CustomerName: "Christin Chukwuma",
		TicketOrder: concertspb.CreateTicketOrder(
			"2349587234",
			time.Now().UnixMilli(),
			concertspb.CreateAddress("Main Street 1", "Suns-ville", "NB", "US"),
			[]*concertspb.Item{
				concertspb.CreateItem("Concert ticket", 2),
				concertspb.CreateItem("Consumption voucher", 4),
			}),
	}
	if err := db.Create(&ticketSale).Error; err != nil {
		return err
	}
	fmt.Printf("Created ticket sale #%v\n", ticketSale.ID)

	// Read the ticket sale back from the database.
	var ts sample_model.TicketSale
	if err := db.First(&ts).Error; err != nil {
		return err
	}
	fmt.Printf("Found ticket sale %v with protobuf value %v\n", ticketSale.ID, ticketSale.TicketOrder)

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
