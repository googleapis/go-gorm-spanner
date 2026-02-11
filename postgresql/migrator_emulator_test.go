// Copyright 2026 Google LLC
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

package spannerpg

import (
	"context"
	"database/sql"
	"log"
	"math/big"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/go-cmp/cmp"
	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/testutil"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
)

type Singer struct {
	gorm.Model
	FirstName sql.NullString
	LastName  string
	FullName  string `gorm:"->;type:varchar generated always as (CASE WHEN first_name IS NULL THEN last_name WHEN last_name  IS NULL THEN first_name ELSE first_name || ' ' || last_name END) stored;default:(-);"`
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
	Description spanner.PGJsonB `gorm:"type:jsonb"`
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

type ScannablePGNumeric spanner.PGNumeric

func (n *ScannablePGNumeric) Scan(value any) error {
	if value == nil {
		n.Numeric, n.Valid = "", false
		return nil
	}
	n.Valid = true
	switch p := value.(type) {
	default:
		return status.Errorf(codes.InvalidArgument, "invalid type for PGNumeric: %v", p)
	case string:
		n.Numeric = p
		n.Valid = true
	case *string:
		n.Numeric = *p
		n.Valid = true
	}
	return nil
}

type AllTypes struct {
	ID           int64
	ColBool      bool
	ColBytes     []byte
	ColDate      civil.Date `gorm:"type:date"`
	ColFloat32   float32
	ColFloat64   float64
	ColInt64     int64
	ColJson      spanner.PGJsonB    `gorm:"type:jsonb"`
	ColNumeric   ScannablePGNumeric `gorm:"type:numeric"`
	ColString    string
	ColTimestamp time.Time
}

type SqlNullTypes struct {
	ID           int64
	ColBool      sql.NullBool
	ColFloat64   sql.NullFloat64
	ColInt64     sql.NullInt64
	ColString    sql.NullString
	ColTimestamp sql.NullTime
}

type AllNullTypes struct {
	ID           int64
	ColBool      spanner.NullBool    `gorm:"type:bool"`
	ColDate      spanner.NullDate    `gorm:"type:date"`
	ColFloat32   spanner.NullFloat32 `gorm:"type:real"`
	ColFloat64   spanner.NullFloat64 `gorm:"type:double precision"`
	ColInt64     spanner.NullInt64   `gorm:"type:bigint"`
	ColJson      spanner.PGJsonB     `gorm:"type:jsonb"`
	ColNumeric   spanner.PGNumeric   `gorm:"type:numeric"`
	ColString    spanner.NullString  `gorm:"type:varchar"`
	ColTimestamp spanner.NullTime    `gorm:"type:timestamp with time zone"`
}

func TestMain(m *testing.M) {
	// Disable client config logging by default.
	if _, found := os.LookupEnv("GOOGLE_CLOUD_SPANNER_DISABLE_LOG_CLIENT_OPTIONS"); !found {
		_ = os.Setenv("GOOGLE_CLOUD_SPANNER_DISABLE_LOG_CLIENT_OPTIONS", "true")
	}

	cleanup, err := testutil.InitIntegrationTests()
	if err != nil {
		log.Fatalf("could not init integration tests: %v", err)
	}
	res := m.Run()
	cleanup()
	os.Exit(res)
}

func skipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in -short mode.")
	}
	if testutil.Skipped {
		t.Skip("Integration tests skipped")
	}
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

	dsn, cleanup, err := testutil.CreateTestDB(context.Background(), databasepb.DatabaseDialect_POSTGRESQL)
	if err != nil {
		log.Fatalf("could not init integration tests while creating database: %v", err)
	}
	defer cleanup()
	// Open db.
	db, err := gorm.Open(New(postgres.Config{
		DSN: dsn,
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		log.Fatal(err)
	}
	tables := []interface{}{&Singer{}, &Album{}, &Track{}, &Venue{}, &Concert{}}
	statements, err := db.Migrator().(spannergorm.SpannerMigrator).AutoMigrateDryRun(tables...)
	if g, w := len(statements), 12; g != w {
		t.Fatalf("num statements mismatch\n Got: %d\nWant: %d", g, w)
	}
	for i, statement := range statements {
		if g, w := len(statement.Params), 0; g != w {
			t.Fatalf("%d: num params mismatch\n Got: %d\nWant: %d", i, g, w)
		}
	}
	if g, w := statements[0].SQL, `alter database "db" set spanner.default_sequence_kind = 'bit_reversed_positive'`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[1].SQL, `CREATE TABLE "singers" ("id" serial,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"first_name" text,"last_name" text,"full_name" varchar generated always as (CASE WHEN first_name IS NULL THEN last_name WHEN last_name  IS NULL THEN first_name ELSE first_name || ' ' || last_name END) stored,"active" boolean,PRIMARY KEY ("id"))`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[2].SQL, `CREATE INDEX IF NOT EXISTS "idx_singers_deleted_at" ON "singers" ("deleted_at")`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[3].SQL, `CREATE TABLE "albums" ("id" serial,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"title" text,"marketing_budget" boolean,"release_date" date,"cover_picture" bytea,"singer_id" int,PRIMARY KEY ("id"),CONSTRAINT "fk_singers_albums" FOREIGN KEY ("singer_id") REFERENCES "singers"("id"))`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[4].SQL, `CREATE INDEX IF NOT EXISTS "idx_albums_deleted_at" ON "albums" ("deleted_at")`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[5].SQL, `CREATE TABLE "tracks" ("id" serial,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"track_number" int,"title" text,"sample_rate" numeric,"album_id" int,PRIMARY KEY ("id"),CONSTRAINT "fk_albums_tracks" FOREIGN KEY ("album_id") REFERENCES "albums"("id"))`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[6].SQL, `CREATE INDEX IF NOT EXISTS "idx_tracks_deleted_at" ON "tracks" ("deleted_at")`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[7].SQL, `CREATE TABLE "venues" ("id" serial,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"name" text,"description" jsonb,PRIMARY KEY ("id"))`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[8].SQL, `CREATE INDEX IF NOT EXISTS "idx_venues_deleted_at" ON "venues" ("deleted_at")`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[9].SQL, `CREATE TABLE "concerts" ("id" serial,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"name" text,"venue_id" int,"singer_id" int,"start_time" timestamptz,"end_time" timestamptz,PRIMARY KEY ("id"),CONSTRAINT "fk_venues_concerts" FOREIGN KEY ("venue_id") REFERENCES "venues"("id"),CONSTRAINT "fk_singers_concerts" FOREIGN KEY ("singer_id") REFERENCES "singers"("id"))`; g != w {
		// The order of foreign key constraints in the generated DDL that is returned by Spanner is non-deterministic.
		if g, w := statements[9].SQL, `CREATE TABLE "concerts" ("id" serial,"created_at" timestamptz,"updated_at" timestamptz,"deleted_at" timestamptz,"name" text,"venue_id" int,"singer_id" int,"start_time" timestamptz,"end_time" timestamptz,PRIMARY KEY ("id"),CONSTRAINT "fk_singers_concerts" FOREIGN KEY ("singer_id") REFERENCES "singers"("id"),CONSTRAINT "fk_venues_concerts" FOREIGN KEY ("venue_id") REFERENCES "venues"("id"))`; g != w {
			t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
		}
	}
	if g, w := statements[10].SQL, `CREATE INDEX IF NOT EXISTS "idx_concerts_time" ON "concerts" ("start_time","end_time")`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}
	if g, w := statements[11].SQL, `CREATE INDEX IF NOT EXISTS "idx_concerts_deleted_at" ON "concerts" ("deleted_at")`; g != w {
		t.Fatalf("SQL mismatch\n Got: %s\nWant: %s", g, w)
	}

	err = db.Migrator().AutoMigrate(tables...)
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
	defer func() { _ = databaseAdminClient.Close() }()
	resp, err := databaseAdminClient.GetDatabaseDdl(context.Background(), &databasepb.GetDatabaseDdlRequest{
		Database: dsn,
	})
	if err != nil {
		t.Fatalf("failed to get database DDL: %v", err)
	}
	if g, w := len(resp.GetStatements()), 12; g != w {
		t.Fatalf("ddl statement count mismatch\n Got: %v\nWant: %v", g, w)
	}
	gotStatements := resp.GetStatements()
	slices.Sort(gotStatements)
	altCreateConcerts := "CREATE TABLE concerts (\n  id bigint GENERATED BY DEFAULT AS IDENTITY NOT NULL,\n  created_at timestamp with time zone,\n  updated_at timestamp with time zone,\n  deleted_at timestamp with time zone,\n  name character varying,\n  venue_id bigint,\n  singer_id bigint,\n  start_time timestamp with time zone,\n  end_time timestamp with time zone,\n  PRIMARY KEY(id),\n  CONSTRAINT fk_venues_concerts FOREIGN KEY (venue_id) REFERENCES venues(id),\n  CONSTRAINT fk_singers_concerts FOREIGN KEY (singer_id) REFERENCES singers(id)\n)"
	for i, ddl := range []string{
		`ALTER DATABASE db SET "spanner.default_sequence_kind" = 'bit_reversed_positive'`,
		"CREATE INDEX idx_albums_deleted_at ON albums (deleted_at)",
		"CREATE INDEX idx_concerts_deleted_at ON concerts (deleted_at)",
		"CREATE INDEX idx_concerts_time ON concerts (start_time, end_time)",
		"CREATE INDEX idx_singers_deleted_at ON singers (deleted_at)",
		"CREATE INDEX idx_tracks_deleted_at ON tracks (deleted_at)",
		"CREATE INDEX idx_venues_deleted_at ON venues (deleted_at)",
		"CREATE TABLE albums (\n  id bigint GENERATED BY DEFAULT AS IDENTITY NOT NULL,\n  created_at timestamp with time zone,\n  updated_at timestamp with time zone,\n  deleted_at timestamp with time zone,\n  title character varying,\n  marketing_budget boolean,\n  release_date date,\n  cover_picture bytea,\n  singer_id bigint,\n  PRIMARY KEY(id),\n  CONSTRAINT fk_singers_albums FOREIGN KEY (singer_id) REFERENCES singers(id)\n)",
		"CREATE TABLE concerts (\n  id bigint GENERATED BY DEFAULT AS IDENTITY NOT NULL,\n  created_at timestamp with time zone,\n  updated_at timestamp with time zone,\n  deleted_at timestamp with time zone,\n  name character varying,\n  venue_id bigint,\n  singer_id bigint,\n  start_time timestamp with time zone,\n  end_time timestamp with time zone,\n  PRIMARY KEY(id),\n  CONSTRAINT fk_singers_concerts FOREIGN KEY (singer_id) REFERENCES singers(id),\n  CONSTRAINT fk_venues_concerts FOREIGN KEY (venue_id) REFERENCES venues(id)\n)",
		"CREATE TABLE singers (\n  id bigint GENERATED BY DEFAULT AS IDENTITY NOT NULL,\n  created_at timestamp with time zone,\n  updated_at timestamp with time zone,\n  deleted_at timestamp with time zone,\n  first_name character varying,\n  last_name character varying,\n  full_name character varying GENERATED ALWAYS AS (CASE WHEN (first_name IS NULL) THEN last_name WHEN (last_name IS NULL) THEN first_name ELSE ((first_name || ' '::text) || last_name) END) STORED,\n  active boolean,\n  PRIMARY KEY(id)\n)",
		"CREATE TABLE tracks (\n  id bigint GENERATED BY DEFAULT AS IDENTITY NOT NULL,\n  created_at timestamp with time zone,\n  updated_at timestamp with time zone,\n  deleted_at timestamp with time zone,\n  track_number bigint,\n  title character varying,\n  sample_rate numeric,\n  album_id bigint,\n  PRIMARY KEY(id),\n  CONSTRAINT fk_albums_tracks FOREIGN KEY (album_id) REFERENCES albums(id)\n)",
		"CREATE TABLE venues (\n  id bigint GENERATED BY DEFAULT AS IDENTITY NOT NULL,\n  created_at timestamp with time zone,\n  updated_at timestamp with time zone,\n  deleted_at timestamp with time zone,\n  name character varying,\n  description jsonb,\n  PRIMARY KEY(id)\n)",
	} {
		if g, w := gotStatements[i], ddl; g != w {
			// Workaround for the fact that the DDL printer prints constraints in non-deterministic order.
			// That means that the result for CREATE TABLE concerts could be one of two variants.
			if i == 8 && g == altCreateConcerts {
				continue
			}
			t.Errorf("%d: ddl mismatch\n Got: %v\nWant: %v", i, g, w)
		}
	}
}

func verifyGetIndexes(t *testing.T, db *gorm.DB) {
	compareIndexFunc := func(a, b gorm.Index) int {
		if a.Table() != b.Table() {
			return strings.Compare(a.Table(), b.Table())
		}
		return strings.Compare(a.Name(), b.Name())
	}

	singerIndexes, err := db.Migrator().GetIndexes("singers")
	slices.SortFunc(singerIndexes, compareIndexFunc)
	if err != nil {
		t.Fatalf("failed to get indexes for singers: %v", err)
	}
	want := []gorm.Index{
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
	}
	if !reflect.DeepEqual(singerIndexes, want) {
		t.Fatalf("singers GetIndexes mismatch: %v", singerIndexes)
	}
	concertIndexes, err := db.Migrator().GetIndexes("concerts")
	slices.SortFunc(concertIndexes, compareIndexFunc)
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

func TestMigrateMultipleTimesUniqueIndexSameFieldName(t *testing.T) {
	skipIfShortOrNotEmulator(t)
	t.Parallel()

	dsn, cleanup, err := testutil.CreateTestDB(context.Background(), databasepb.DatabaseDialect_POSTGRESQL)
	if err != nil {
		log.Fatalf("could not init integration tests while creating database: %v", err)
	}
	defer cleanup()
	// Open db.
	db, err := gorm.Open(New(postgres.Config{
		DSN: dsn,
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		log.Fatal(err)
	}

	// A struct that has a SHA256 field.
	type As struct {
		gorm.Model
		SHA256 string
	}

	// Another struct with a SHA256 field, but it is unique.
	type Bs struct {
		gorm.Model
		SHA256 string `gorm:"uniqueIndex"`
	}

	if err := db.AutoMigrate(&As{}, &Bs{}); err != nil {
		t.Fatalf("Failed first migrate, got error: %v", err)
	}

	// Ensure the `as` table is able to migrate a second time.
	// Prior to the bug fix accompanying this test, the `as` table would fail to
	// be migrated with this error:
	// `NotFound desc = uni_as_sha256 is not a constraint in as`
	// The migrator was trying to drop the unique constraint on the SHA256 field
	// of the `as` table, which doesn't exist. This was happening because the
	// get column details query was crossing table boundaries and misattributing
	// the uniqueness of the SHA256 column, with the `as` table.
	if err := db.AutoMigrate(&As{}); err != nil {
		t.Fatalf("Failed second migrate, got error: %v", err)
	}
}

func TestMigrateMultipleTimesUniqueIndex(t *testing.T) {
	skipIfShortOrNotEmulator(t)
	t.Parallel()

	dsn, cleanup, err := testutil.CreateTestDB(context.Background(), databasepb.DatabaseDialect_POSTGRESQL)
	if err != nil {
		log.Fatalf("could not init integration tests while creating database: %v", err)
	}
	defer cleanup()
	// Open db.
	db, err := gorm.Open(New(postgres.Config{
		DSN: dsn,
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		log.Fatal(err)
	}

	// A struct that has a unique index.
	type As struct {
		gorm.Model
		SHA256 string `gorm:"uniqueIndex"`
	}

	if err := db.AutoMigrate(&As{}); err != nil {
		t.Fatalf("Failed first migrate, got error: %v", err)
	}

	if err := db.AutoMigrate(&As{}); err != nil {
		t.Fatalf("Failed second migrate, got error: %v", err)
	}
}

func TestMigrateUniqueFieldFails(t *testing.T) {
	skipIfShortOrNotEmulator(t)
	t.Parallel()

	dsn, cleanup, err := testutil.CreateTestDB(context.Background(), databasepb.DatabaseDialect_POSTGRESQL)
	if err != nil {
		log.Fatalf("could not init integration tests while creating database: %v", err)
	}
	defer cleanup()
	// Open db.
	db, err := gorm.Open(New(postgres.Config{
		DSN: dsn,
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		log.Fatal(err)
	}

	// A struct that has a unique field.
	type As struct {
		gorm.Model
		SHA256 string `gorm:"unique"`
	}

	errString := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}
	if g, w := errString(db.AutoMigrate(&As{})), "rpc error: code = FailedPrecondition desc = <UNIQUE> constraint is not supported, create a unique index instead."; g != w {
		t.Fatalf("first migrate error mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := errString(db.AutoMigrate(&As{})), "rpc error: code = FailedPrecondition desc = <UNIQUE> constraint is not supported, create a unique index instead."; g != w {
		t.Fatalf("second migrate error mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestMigrateAllTypes(t *testing.T) {
	skipIfShortOrNotEmulator(t)
	t.Parallel()

	dsn, cleanup, err := testutil.CreateTestDB(context.Background(), databasepb.DatabaseDialect_POSTGRESQL)
	if err != nil {
		log.Fatalf("could not init integration tests while creating database: %v", err)
	}
	defer cleanup()
	// Open db.
	db, err := gorm.Open(New(postgres.Config{
		DSN: dsn,
	}), &gorm.Config{PrepareStmt: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&AllTypes{}); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&SqlNullTypes{}); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&AllNullTypes{}); err != nil {
		t.Fatal(err)
	}
	verifyColumnType(t, db, "all_types", "col_bool", "boolean")
	verifyColumnType(t, db, "all_types", "col_bytes", "bytea")
	verifyColumnType(t, db, "all_types", "col_date", "date")
	// PG gorm by default maps Golang's float32 to numeric columns.
	verifyColumnType(t, db, "all_types", "col_float32", "numeric")
	verifyColumnType(t, db, "all_types", "col_float64", "numeric")
	verifyColumnType(t, db, "all_types", "col_int64", "bigint")
	verifyColumnType(t, db, "all_types", "col_json", "jsonb")
	verifyColumnType(t, db, "all_types", "col_numeric", "numeric")
	verifyColumnType(t, db, "all_types", "col_string", "character varying")
	verifyColumnType(t, db, "all_types", "col_timestamp", "timestamp with time zone")

	verifyColumnType(t, db, "sql_null_types", "col_bool", "boolean")
	verifyColumnType(t, db, "sql_null_types", "col_float64", "numeric")
	verifyColumnType(t, db, "sql_null_types", "col_int64", "bigint")
	verifyColumnType(t, db, "sql_null_types", "col_string", "character varying")
	verifyColumnType(t, db, "sql_null_types", "col_timestamp", "timestamp with time zone")

	verifyColumnType(t, db, "all_null_types", "col_bool", "boolean")
	verifyColumnType(t, db, "all_null_types", "col_date", "date")
	verifyColumnType(t, db, "all_null_types", "col_float32", "real")
	verifyColumnType(t, db, "all_null_types", "col_float64", "double precision")
	verifyColumnType(t, db, "all_null_types", "col_int64", "bigint")
	verifyColumnType(t, db, "all_null_types", "col_json", "jsonb")
	verifyColumnType(t, db, "all_null_types", "col_numeric", "numeric")
	verifyColumnType(t, db, "all_null_types", "col_string", "character varying")
	verifyColumnType(t, db, "all_null_types", "col_timestamp", "timestamp with time zone")

	// Verify that we can insert and retrieve data for all the above data types.
	timeValue, err := time.Parse(time.RFC3339, "2025-02-17T11:14:00+01:00")
	if err != nil {
		t.Fatalf("could not parse time: %v", err)
	}
	rowAllTypes := AllTypes{
		ID:           1,
		ColBool:      true,
		ColBytes:     []byte("test"),
		ColDate:      civil.Date{Year: 2025, Month: 2, Day: 17},
		ColFloat32:   float32(3.14),
		ColFloat64:   3.14,
		ColInt64:     int64(-1),
		ColJson:      spanner.PGJsonB{Value: "{\"key\": \"value\"}", Valid: true},
		ColNumeric:   ScannablePGNumeric{Numeric: "3.14", Valid: true},
		ColString:    "test",
		ColTimestamp: timeValue,
	}
	if err := db.Create(&rowAllTypes).Error; err != nil {
		t.Fatalf("could not insert AllTypes row: %v", err)
	}

	rowSqlNullTypesWithoutValues := SqlNullTypes{ID: 1}
	if err := db.Create(&rowSqlNullTypesWithoutValues).Error; err != nil {
		t.Fatalf("could not insert SqlNullTypes row without values: %v", err)
	}
	rowSqlNullTypes := SqlNullTypes{
		ID:           2,
		ColBool:      sql.NullBool{Bool: rowAllTypes.ColBool, Valid: true},
		ColFloat64:   sql.NullFloat64{Float64: rowAllTypes.ColFloat64, Valid: true},
		ColInt64:     sql.NullInt64{Int64: rowAllTypes.ColInt64, Valid: true},
		ColString:    sql.NullString{String: rowAllTypes.ColString, Valid: true},
		ColTimestamp: sql.NullTime{Time: rowAllTypes.ColTimestamp, Valid: true},
	}
	if err := db.Create(&rowSqlNullTypes).Error; err != nil {
		t.Fatalf("could not insert SqlNullTypes row with values: %v", err)
	}

	rowAllNullTypesWithoutValues := AllNullTypes{ID: 1}
	if err := db.Create(&rowAllNullTypesWithoutValues).Error; err != nil {
		t.Fatalf("could not insert AllNullTypes row without values: %v", err)
	}
	rowAllNullTypes := AllNullTypes{
		ID:           2,
		ColBool:      spanner.NullBool{Bool: rowAllTypes.ColBool, Valid: true},
		ColDate:      spanner.NullDate{Date: rowAllTypes.ColDate, Valid: true},
		ColFloat32:   spanner.NullFloat32{Float32: rowAllTypes.ColFloat32, Valid: true},
		ColFloat64:   spanner.NullFloat64{Float64: rowAllTypes.ColFloat64, Valid: true},
		ColInt64:     spanner.NullInt64{Int64: rowAllTypes.ColInt64, Valid: true},
		ColJson:      spanner.PGJsonB{Value: rowAllTypes.ColJson.Value, Valid: true},
		ColNumeric:   spanner.PGNumeric{Numeric: rowAllTypes.ColNumeric.Numeric, Valid: true},
		ColString:    spanner.NullString{StringVal: rowAllTypes.ColString, Valid: true},
		ColTimestamp: spanner.NullTime{Time: rowAllTypes.ColTimestamp, Valid: true},
	}
	if err := db.Create(&rowAllNullTypes).Error; err != nil {
		t.Fatalf("could not insert AllNullTypes row with values: %v", err)
	}

	// Verify that we can get all types.
	var rowAllTypesRead AllTypes
	if err := db.Find(&rowAllTypesRead, 1).Error; err != nil {
		t.Fatalf("could not read AllTypes: %v", err)
	}
	if g, w := rowAllTypesRead, rowAllTypes; !cmp.Equal(g, w, cmp.Comparer(func(n1, n2 big.Rat) bool {
		return spanner.NumericString(&n1) == spanner.NumericString(&n2)
	})) {
		t.Fatalf("AllTypes row mismatch\n Got: %v\nWant: %v", g, w)
	}

	var rowSqlNullTypesWithoutValuesRead SqlNullTypes
	if err := db.Find(&rowSqlNullTypesWithoutValuesRead, 1).Error; err != nil {
		t.Fatalf("could not read SqlNullTypes without values: %v", err)
	}
	if g, w := rowSqlNullTypesWithoutValuesRead, rowSqlNullTypesWithoutValues; !cmp.Equal(g, w) {
		t.Fatalf("SqlNullTypes without values row mismatch\n Got: %v\nWant: %v", g, w)
	}
	var rowSqlNullTypesRead SqlNullTypes
	if err := db.Find(&rowSqlNullTypesRead, 2).Error; err != nil {
		t.Fatalf("could not read SqlNullTypes: %v", err)
	}
	if g, w := rowSqlNullTypesRead, rowSqlNullTypes; !cmp.Equal(g, w) {
		t.Fatalf("SqlNullTypes row mismatch\n Got: %v\nWant: %v", g, w)
	}

	var rowAllNullTypesWithoutValuesRead AllNullTypes
	if err := db.Find(&rowAllNullTypesWithoutValuesRead, 1).Error; err != nil {
		t.Fatalf("could not read AllNullTypes without values: %v", err)
	}
	if g, w := rowAllNullTypesWithoutValuesRead, rowAllNullTypesWithoutValues; !cmp.Equal(g, w, cmp.Comparer(func(n1, n2 spanner.NullNumeric) bool {
		return n1.Valid == false && n2.Valid == false
	})) {
		t.Fatalf("AllNullTypes without values row mismatch\n Got: %v\nWant: %v", g, w)
	}
	var rowAllNullTypesRead AllNullTypes
	if err := db.Find(&rowAllNullTypesRead, 2).Error; err != nil {
		t.Fatalf("could not read AllNullTypes: %v", err)
	}
	if g, w := rowAllNullTypesRead, rowAllNullTypes; !cmp.Equal(g, w, cmp.Comparer(func(n1, n2 spanner.NullNumeric) bool {
		return spanner.NumericString(&n1.Numeric) == spanner.NumericString(&n2.Numeric)
	})) {
		t.Fatalf("AllNullTypes row mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func verifyColumnType(t *testing.T, db *gorm.DB, table, column, want string) {
	query := "select spanner_type from information_schema.columns where table_name=? and column_name=?"
	row := db.ConnPool.QueryRowContext(context.Background(), query, table, column)
	if row == nil {
		t.Fatalf("no column %v.%v found", table, column)
	}
	if err := row.Err(); err != nil {
		t.Fatalf("failed to get data type of column %v.%v: %v", table, column, err)
	}
	var tp string
	if err := row.Scan(&tp); err != nil {
		t.Fatalf("failed to scan data type of column %v.%v: %v", table, column, err)
	}
	if g, w := tp, want; g != w {
		t.Fatalf("data type mismatch for column %v.%v\n Got: %v\nWant: %v", table, column, g, w)
	}
}
