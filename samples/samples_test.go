// Copyright 2023 Google LLC.
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

package samples_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	spannergorm "github.com/googleapis/go-gorm-spanner"
	"github.com/googleapis/go-gorm-spanner/samples/interleave"
	"github.com/googleapis/go-gorm-spanner/testutil"
)

type sampleFunc func(w io.Writer, db *gorm.DB) error

func TestIntegration_Sample(t *testing.T) {
	cleanup, err := testutil.InitIntegrationTests()
	require.NoError(t, err)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	dsn, cleanup, err := testutil.CreateTestDB(ctx)
	require.NoError(t, err)
	defer cleanup()
	// Open db.
	db, err := gorm.Open(spannergorm.New(spannergorm.Config{
		DriverName: "spanner",
		DSN:        dsn,
	}), &gorm.Config{PrepareStmt: true, IgnoreRelationshipsWhenMigrating: true, Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	require.NoError(t, samples.CreateInterleavedTablesIfNotExist(os.Stdout, db))
	require.NoError(t, db.AutoMigrate(&samples.Venue{}, &samples.Concert{}))
	require.NoError(t, samples.DeleteAllData(db))
	var out string
	out = runSample(t, samples.CreateRandomSingersAndAlbums, db, "failed to create singers and albums")
	assertContains(t, out, "Created random singers and albums")
	out = runSample(t, samples.PrintSingersAlbumsAndTracks, db, "failed to print singers, albums and tracks")
	assertContains(t, out, "Fetched all singers, albums and tracks")
	out = runSample(t, samples.CreateVenueAndConcertInTransaction, db, "failed to create venue and concert in transaction")
	assertContains(t, out, "Created a Venue and a Concert")
	out = runSample(t, samples.PrintConcerts, db, "failed to fetch concerts")
	assertContains(t, out, "Fetched all concerts")
	out = runSample(t, samples.PrintAlbumsReleaseBefore1900, db, "failed to fetch albums released before 1900")
	assertContains(t, out, "was released at")
}

func runSample(t *testing.T, f sampleFunc, db *gorm.DB, errMsg string) string {
	var b bytes.Buffer
	if err := f(&b, db); err != nil {
		t.Errorf("%s: %v", errMsg, err)
	}
	return b.String()
}

func assertContains(t *testing.T, out string, sub string) {
	t.Helper()
	if !strings.Contains(out, sub) {
		t.Errorf("got output %q; want it to contain %q", out, sub)
	}
}

func assertNotContains(t *testing.T, out string, sub string) {
	t.Helper()
	if strings.Contains(out, sub) {
		t.Errorf("got output %q; want it to not contain %q", out, sub)
	}
}
