// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gorm

import (
	"context"
	"database/sql"
	"log"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/googleapis/go-gorm-spanner/testutil"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
)

type Singer struct {
	gorm.Model
	FirstName sql.NullString
	LastName  string
	FullName  string `gorm:"->;type:STRING(MAX) AS (concat(coalesce(first_name, ''),' ',last_name)) STORED;default:(-);"`
	Active    bool
	Albums    []Album
	Concerts  []Concert
}

type Album struct {
	gorm.Model
	Title           string
	MarketingBudget decimal.NullDecimal
	ReleaseDate     datatypes.Date
	CoverPicture    []byte
	Singer          Singer
	SingerId        int64
	Tracks          []Track
}

type Track struct {
	gorm.Model
	TrackNumber int64
	Title       string
	SampleRate  float64
	Album       Album
	AlbumId     int64
}

type Venue struct {
	gorm.Model
	Name        string
	Description spanner.NullJSON
	Concerts    []Concert
}

type Concert struct {
	gorm.Model
	Name      string
	Venue     Venue
	VenueId   int64
	Singer    Singer
	SingerId  int64
	StartTime time.Time `gorm:"index:idx_concerts_time"`
	EndTime   time.Time `gorm:"index:idx_concerts_time"`
}

// The tests in this file are only executed on the emulator, as they would be relatively slow
// on Cloud Spanner.

func skipIfShortOrNotEmulator(t *testing.T) {
	skipIfShort(t)
	if !testutil.RunsOnEmulator() {
		t.Skip("These tests are only executed on the emulator")
	}
}

func TestAutoMigrate_CreateDataModel(t *testing.T) {
	skipIfShortOrNotEmulator(t)
	t.Parallel()

	dsn, cleanup, err := testutil.CreateTestDB(context.Background())
	if err != nil {
		log.Fatalf("could not init integration tests while creating database: %v", err)
	}
	defer cleanup()
	// Open db.
	db, err := gorm.Open(New(Config{
		DriverName: "spanner",
		DSN:        dsn,
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		log.Fatal(err)
	}
	err = db.Migrator().AutoMigrate(&Singer{}, &Album{}, &Track{}, &Venue{}, &Concert{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the database schema.
	verifyDatabaseSchema(t, dsn)

	// Verify that we can run AutoMigrate again without any problems.
	err = db.Migrator().AutoMigrate(&Singer{}, &Album{}, &Track{}, &Venue{}, &Concert{})
	if err != nil {
		t.Fatal(err)
	}
	verifyDatabaseSchema(t, dsn)

	// Verify the return value of GetIndexes.
	verifyGetIndexes(t, db)

	// Verify that we can manually create an index.
	type singer struct {
		LastName string `gorm:"index:idx_singers_last_name"`
	}
	if db.Migrator().HasIndex(&singer{}, "idx_singers_last_name") {
		t.Fatalf("singers already has an index on last_name")
	}
	if err := db.Migrator().CreateIndex(&singer{}, "idx_singers_last_name"); err != nil {
		t.Fatalf("failed to create index on singers.last_name: %v", err)
	}
	if !db.Migrator().HasIndex(&singer{}, "idx_singers_last_name") {
		t.Fatalf("idx_singers_last_name not found")
	}
}

func verifyDatabaseSchema(t *testing.T, dsn string) {
	databaseAdminClient, err := database.NewDatabaseAdminClient(context.Background())
	if err != nil {
		t.Fatalf("failed to open database admin client: %v", err)
	}
	defer databaseAdminClient.Close()
	resp, err := databaseAdminClient.GetDatabaseDdl(context.Background(), &databasepb.GetDatabaseDdlRequest{
		Database: dsn,
	})
	if err != nil {
		t.Fatalf("failed to get database DDL: %v", err)
	}
	if g, w := len(resp.GetStatements()), 16; g != w {
		t.Errorf("ddl statement count mismatch\n Got: %v\nWant: %v", g, w)
	}
	altCreateConcerts := "CREATE TABLE concerts (\n  id INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence concerts_seq)),\n  created_at TIMESTAMP,\n  updated_at TIMESTAMP,\n  deleted_at TIMESTAMP,\n  name STRING(MAX),\n  venue_id INT64,\n  singer_id INT64,\n  start_time TIMESTAMP,\n  end_time TIMESTAMP,\n  CONSTRAINT fk_singers_concerts FOREIGN KEY(singer_id) REFERENCES singers(id),\n  CONSTRAINT fk_venues_concerts FOREIGN KEY(venue_id) REFERENCES venues(id),\n) PRIMARY KEY(id)"
	for i, ddl := range []string{
		"CREATE SEQUENCE singers_seq OPTIONS (\n  sequence_kind = 'bit_reversed_positive' )",
		"CREATE SEQUENCE albums_seq OPTIONS (\n  sequence_kind = 'bit_reversed_positive' )",
		"CREATE SEQUENCE tracks_seq OPTIONS (\n  sequence_kind = 'bit_reversed_positive' )",
		"CREATE SEQUENCE venues_seq OPTIONS (\n  sequence_kind = 'bit_reversed_positive' )",
		"CREATE SEQUENCE concerts_seq OPTIONS (\n  sequence_kind = 'bit_reversed_positive' )",
		"CREATE TABLE singers (\n  id INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence singers_seq)),\n  created_at TIMESTAMP,\n  updated_at TIMESTAMP,\n  deleted_at TIMESTAMP,\n  first_name STRING(MAX),\n  last_name STRING(MAX),\n  full_name STRING(MAX) AS (concat(coalesce(first_name, ''),' ',last_name)) STORED,\n  active BOOL,\n) PRIMARY KEY(id)",
		"CREATE INDEX idx_singers_deleted_at ON singers(deleted_at)",
		"CREATE TABLE albums (\n  id INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence albums_seq)),\n  created_at TIMESTAMP,\n  updated_at TIMESTAMP,\n  deleted_at TIMESTAMP,\n  title STRING(MAX),\n  marketing_budget BOOL,\n  release_date DATE,\n  cover_picture BYTES(MAX),\n  singer_id INT64,\n  CONSTRAINT fk_singers_albums FOREIGN KEY(singer_id) REFERENCES singers(id),\n) PRIMARY KEY(id)",
		"CREATE INDEX idx_albums_deleted_at ON albums(deleted_at)",
		"CREATE TABLE tracks (\n  id INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence tracks_seq)),\n  created_at TIMESTAMP,\n  updated_at TIMESTAMP,\n  deleted_at TIMESTAMP,\n  track_number INT64,\n  title STRING(MAX),\n  sample_rate FLOAT64,\n  album_id INT64,\n  CONSTRAINT fk_albums_tracks FOREIGN KEY(album_id) REFERENCES albums(id),\n) PRIMARY KEY(id)",
		"CREATE INDEX idx_tracks_deleted_at ON tracks(deleted_at)",
		"CREATE TABLE venues (\n  id INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence venues_seq)),\n  created_at TIMESTAMP,\n  updated_at TIMESTAMP,\n  deleted_at TIMESTAMP,\n  name STRING(MAX),\n  description JSON,\n) PRIMARY KEY(id)",
		"CREATE INDEX idx_venues_deleted_at ON venues(deleted_at)",
		"CREATE TABLE concerts (\n  id INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence concerts_seq)),\n  created_at TIMESTAMP,\n  updated_at TIMESTAMP,\n  deleted_at TIMESTAMP,\n  name STRING(MAX),\n  venue_id INT64,\n  singer_id INT64,\n  start_time TIMESTAMP,\n  end_time TIMESTAMP,\n  CONSTRAINT fk_venues_concerts FOREIGN KEY(venue_id) REFERENCES venues(id),\n  CONSTRAINT fk_singers_concerts FOREIGN KEY(singer_id) REFERENCES singers(id),\n) PRIMARY KEY(id)",
		"CREATE INDEX idx_concerts_deleted_at ON concerts(deleted_at)",
		"CREATE INDEX idx_concerts_time ON concerts(start_time, end_time)",
	} {
		if g, w := resp.GetStatements()[i], ddl; g != w {
			// Workaround for the fact that the DDL printer prints constraints in non-deterministic order.
			// That means that the result for CREATE TABLE concerts could be one of two variants.
			if i == 13 && g == altCreateConcerts {
				continue
			}
			t.Errorf("%d: ddl mismatch\n Got: %v\nWant: %v", i, g, w)
		}
	}
}

func verifyGetIndexes(t *testing.T, db *gorm.DB) {
	singerIndexes, err := db.Migrator().GetIndexes("singers")
	if err != nil {
		t.Fatalf("failed to get indexes for singers: %v", err)
	}
	if !reflect.DeepEqual(singerIndexes, []gorm.Index{
		&migrator.Index{
			TableName:       "singers",
			NameValue:       "PRIMARY_KEY",
			UniqueValue:     sql.NullBool{Valid: true, Bool: true},
			PrimaryKeyValue: sql.NullBool{Valid: true, Bool: true},
			ColumnList:      []string{"id"},
		},
		&migrator.Index{
			TableName:       "singers",
			NameValue:       "idx_singers_deleted_at",
			UniqueValue:     sql.NullBool{Valid: true, Bool: false},
			PrimaryKeyValue: sql.NullBool{Valid: true, Bool: false},
			ColumnList:      []string{"deleted_at"},
		},
	}) {
		t.Fatalf("singers GetIndexes mismatch: %v", singerIndexes)
	}
	concertIndexes, err := db.Migrator().GetIndexes("concerts")
	if err != nil {
		t.Fatalf("failed to get indexes for concerts: %v", err)
	}
	if !reflect.DeepEqual(concertIndexes, []gorm.Index{
		&migrator.Index{
			TableName:       "concerts",
			NameValue:       "PRIMARY_KEY",
			UniqueValue:     sql.NullBool{Valid: true, Bool: true},
			PrimaryKeyValue: sql.NullBool{Valid: true, Bool: true},
			ColumnList:      []string{"id"},
		},
		&migrator.Index{
			TableName:       "concerts",
			NameValue:       "idx_concerts_deleted_at",
			UniqueValue:     sql.NullBool{Valid: true, Bool: false},
			PrimaryKeyValue: sql.NullBool{Valid: true, Bool: false},
			ColumnList:      []string{"deleted_at"},
		},
		&migrator.Index{
			TableName:       "concerts",
			NameValue:       "idx_concerts_time",
			UniqueValue:     sql.NullBool{Valid: true, Bool: false},
			PrimaryKeyValue: sql.NullBool{Valid: true, Bool: false},
			ColumnList:      []string{"start_time", "end_time"},
		},
	}) {
		t.Fatalf("concerts GetIndexes mismatch: %v", concertIndexes)
	}
}
