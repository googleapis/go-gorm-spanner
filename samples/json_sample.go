// Copyright 2024 Google LLC
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

package samples

import (
	"fmt"
	"gorm.io/gorm"
	"io"
	"time"

	"cloud.google.com/go/spanner"
	samples "github.com/googleapis/go-gorm-spanner/samples/interleave"
)

func UpdateDataWithJsonColumn(w io.Writer, db *gorm.DB) error {
	if err := db.Save(&samples.Venue{
		Model:       gorm.Model{ID: 4, CreatedAt: time.Now()},
		Name:        "Venue 1",
		Description: "Venue 1 description",
		VenueDetails: spanner.NullJSON{Value: []samples.VenueDetails{
			{Name: spanner.NullString{StringVal: "room1", Valid: true}, Open: true},
			{Name: spanner.NullString{StringVal: "room2", Valid: true}, Open: false},
		}, Valid: true},
	}).Error; err != nil {
		return err
	}
	if err := db.Save(&samples.Venue{
		Model:       gorm.Model{ID: 19, CreatedAt: time.Now()},
		Name:        "Venue 2",
		Description: "Venue 2 description",
		VenueDetails: spanner.NullJSON{Value: samples.VenueDetails{
			Rating: spanner.NullFloat64{Float64: 9, Valid: true},
			Open:   true,
		}, Valid: true},
	}).Error; err != nil {
		return err
	}
	if err := db.Save(&samples.Venue{
		Model:       gorm.Model{ID: 42, CreatedAt: time.Now()},
		Name:        "Venue 3",
		Description: "Venue 3 description",
		VenueDetails: spanner.NullJSON{Value: samples.VenueDetails{
			Name: spanner.NullString{Valid: false},
			Open: map[string]bool{"monday": true, "tuesday": false},
			Tags: []spanner.NullString{{StringVal: "large", Valid: true}, {StringVal: "airy", Valid: true}},
		}, Valid: true},
	}).Error; err != nil {
		return err
	}
	fmt.Fprintf(w, "Updated data to VenueDetails column\n")
	return nil
}

func QueryWithJsonParameter(w io.Writer, db *gorm.DB) error {
	var venues []samples.Venue
	if err := db.Find(&venues, "JSON_VALUE(venue_details, '$.rating') = JSON_VALUE(@details, '$.rating')", map[string]interface{}{
		"details": spanner.NullJSON{Value: samples.VenueDetails{
			Rating: spanner.NullFloat64{Float64: 9, Valid: true},
		}, Valid: true},
	}).Error; err != nil {
		return err
	}
	fmt.Fprintf(w, "The venue details for venue id %v is %v\n", venues[0].ID, venues[0].VenueDetails)
	return nil
}
