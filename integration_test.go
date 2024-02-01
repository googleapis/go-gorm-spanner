// Copyright 2023 Google LLC
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
	"log"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/googleapis/go-gorm-spanner/testutil"
)

func TestMain(m *testing.M) {
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

func TestIntegration_DefaultValue(t *testing.T) {
	skipIfShort(t)
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

	type Harumph struct {
		gorm.Model

		Email   string    `gorm:"not null;index:,unique"`
		Name    string    `gorm:"notNull;default:foo"`
		Name2   string    `gorm:"size:233;not null;default:'foo'"`
		Name3   string    `gorm:"size:233;notNull;default:''"`
		Age     int       `gorm:"default:18"`
		Created time.Time `gorm:"default:2000-01-02"`
		Enabled bool      `gorm:"default:true"`
	}

	if err := db.AutoMigrate(&Harumph{}); err != nil {
		t.Fatalf("Failed to migrate with default value, got error: %v", err)
	}

	harumph := Harumph{Email: "hello@gorm.io"}
	if err := db.Create(&harumph).Error; err != nil {
		t.Fatalf("Failed to create data with default value, got error: %v", err)
	} else if harumph.Name != "foo" || harumph.Name2 != "foo" || harumph.Name3 != "" || harumph.Age != 18 || !harumph.Enabled {
		t.Fatalf("Failed to create data with default value, got: %+v", harumph)
	}

	var result Harumph
	if err := db.First(&result, "email = ?", "hello@gorm.io").Error; err != nil {
		t.Fatalf("Failed to find created data, got error: %v", err)
	} else if result.Name != "foo" || result.Name2 != "foo" || result.Name3 != "" || result.Age != 18 || !result.Enabled || result.Created.Format("20060102") != "20000102" {
		t.Fatalf("Failed to find created data with default data, got %+v", result)
	}
	require.Conditionf(t, func() (success bool) {
		return result.ID > 0
	}, "ID should be greater than 0")
}

func TestIntegration_Distinct(t *testing.T) {
	skipIfShort(t)
	if testutil.RunsOnEmulator() {
		t.Skip("ON DELETE CASCADE is not supported on the emulator")
	}

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
	if err := db.AutoMigrate(&testutil.User{}, &testutil.Account{}, &testutil.Pet{}, &testutil.Company{}, &testutil.Toy{}, &testutil.Language{},
		&testutil.Coupon{}, &testutil.CouponProduct{}, &testutil.Order{}, &testutil.Parent{}, &testutil.Child{}); err != nil {
		t.Fatalf("Failed to migrate models, got error: %v", err)
	}

	users := []testutil.User{
		*testutil.GetUser("distinct", testutil.Config{}),
		*testutil.GetUser("distinct", testutil.Config{}),
		*testutil.GetUser("distinct", testutil.Config{}),
		*testutil.GetUser("distinct-2", testutil.Config{}),
		*testutil.GetUser("distinct-3", testutil.Config{}),
	}
	users[0].Age = 20

	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("errors happened when create users: %v", err)
	}

	var names []string
	db.Table("users").Where("name like ?", "distinct%").Order("name").Pluck("name", &names)
	require.True(t, reflect.DeepEqual(names, []string{"distinct", "distinct", "distinct", "distinct-2", "distinct-3"}))

	var names1 []string
	db.Model(&testutil.User{}).Where("name like ?", "distinct%").Distinct().Order("name").Pluck("Name", &names1)

	require.True(t, reflect.DeepEqual(names1, []string{"distinct", "distinct-2", "distinct-3"}))

	var names2 []string
	db.Scopes(func(db *gorm.DB) *gorm.DB {
		return db.Table("users")
	}).Where("name like ?", "distinct%").Order("name").Pluck("name", &names2)
	require.True(t, reflect.DeepEqual(names2, []string{"distinct", "distinct", "distinct", "distinct-2", "distinct-3"}))

	var results []testutil.User
	if err := db.Distinct("name", "age").Where("name like ?", "distinct%").Order("name, age desc").Find(&results).Error; err != nil {
		t.Errorf("failed to query users, got error: %v", err)
	}

	expects := []testutil.User{
		{Name: "distinct", Age: 20},
		{Name: "distinct", Age: 18},
		{Name: "distinct-2", Age: 18},
		{Name: "distinct-3", Age: 18},
	}

	if len(results) != len(expects) {
		t.Fatalf("invalid results length found, expects: %v, got %v", len(expects), len(results))
	}
	require.True(t, reflect.DeepEqual(results, expects))

	var count int64
	if err := db.Model(&testutil.User{}).Where("name like ?", "distinct%").Count(&count).Error; err != nil || count != 5 {
		t.Errorf("failed to query users count, got error: %v, count: %v", err, count)
	}

	if err := db.Model(&testutil.User{}).Distinct("name").Where("name like ?", "distinct%").Count(&count).Error; err != nil || count != 3 {
		t.Errorf("failed to query users count, got error: %v, count %v", err, count)
	}

	// test for distinct with select
	dryDB := db.Session(&gorm.Session{DryRun: true})
	r := dryDB.Distinct("u.id, u.*").Table("user_speaks as s").Joins("inner join users as u on u.id = s.user_id").Where("s.language_code ='US' or s.language_code ='ES'").Find(&testutil.User{})
	if !regexp.MustCompile(`SELECT DISTINCT u\.id, u\.\* FROM user_speaks as s inner join users as u`).MatchString(r.Statement.SQL.String()) {
		t.Fatalf("Build Distinct with u.*, but got %v", r.Statement.SQL.String())
	}
}
