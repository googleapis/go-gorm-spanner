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
	_ "embed"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	pgsamples "github.com/googleapis/go-gorm-spanner/postgresql/samples/sample_application"
	"github.com/googleapis/go-gorm-spanner/postgresql/samples/snippets"
	"github.com/googleapis/go-gorm-spanner/samples/emulator"
)

//go:embed snippets/sample_model/data_model.sql
var createDataModelSQL string

func main() {
	// Run the larger sample application.
	if len(os.Args) == 1 {
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_POSTGRESQL, func(project string, instance string, database string) error {
			return pgsamples.RunSample(os.Stdout, "projects/"+project+"/instances/"+instance+"/databases/"+database)
		})
		return
	}

	// Get the DDL statements for the sample data model.
	ddlStatements := strings.FieldsFunc(createDataModelSQL, func(r rune) bool {
		return r == ';'
	})
	// Skip the last (empty) statement.
	ddlStatements = ddlStatements[0 : len(ddlStatements)-1]
	// Run one of the sample snippets.
	sample := os.Args[1]

	switch sample {
	case "hello_world":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.HelloWorld, nil, ddlStatements...)
	case "insert_data":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.InsertData, nil, ddlStatements...)
	case "upsert":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.Upsert, nil, ddlStatements...)
	case "batch_insert":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.CreateInBatches, nil, ddlStatements...)
	case "find_in_batches":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.FindInBatches, nil, ddlStatements...)
	case "batch_dml":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.BatchDml, nil, ddlStatements...)
	case "auto_save_associations":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.AutoSaveAssociations, nil, ddlStatements...)
	case "interleaved_tables":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.InterleavedTables, nil, ddlStatements...)
	case "read_only_transaction":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.ReadOnlyTransaction, nil, ddlStatements...)
	case "read_write_transaction":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.ReadWriteTransaction, nil, ddlStatements...)
	case "aborted_transaction":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.AbortedTransaction, nil, ddlStatements...)
	case "migrations":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_POSTGRESQL, snippets.Migrations)
	case "client_library":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.ClientLibrary, nil, ddlStatements...)
	case "uuid_primary_key":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_POSTGRESQL, snippets.UuidPrimaryKey)
	case "bit_reversed_sequence":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_POSTGRESQL, snippets.BitReversedSequence)
	case "array_data_type":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.ArrayDataType, nil, ddlStatements...)
	case "custom_spanner_config":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_POSTGRESQL, snippets.CustomSpannerConfig)
	case "data_types":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_POSTGRESQL, snippets.DataTypes)
	case "last_statement":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.LastStatement, nil, ddlStatements...)
	case "isolation_level":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_POSTGRESQL, snippets.IsolationLevel, nil, ddlStatements...)
	default:
		fmt.Printf("unknown sample: %s\n", sample)
		os.Exit(1)
	}
}
