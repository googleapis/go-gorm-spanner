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

package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/googleapis/go-gorm-spanner/samples/emulator"
	samples "github.com/googleapis/go-gorm-spanner/samples/sample_application"
	"github.com/googleapis/go-gorm-spanner/samples/snippets"
)

//go:embed snippets/sample_model/data_model.sql
var createDataModelSQL string

//go:embed snippets/sample_model/protos/order.pb
var protoDescriptors []byte

func main() {
	// Run the larger sample application.
	if len(os.Args) == 1 {
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, func(project string, instance string, database string) error {
			return samples.RunSample(os.Stdout, "projects/"+project+"/instances/"+instance+"/databases/"+database)
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
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.HelloWorld, protoDescriptors, ddlStatements...)
	case "insert_data":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.InsertData, protoDescriptors, ddlStatements...)
	case "upsert":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.Upsert, protoDescriptors, ddlStatements...)
	case "batch_insert":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.CreateInBatches, protoDescriptors, ddlStatements...)
	case "find_in_batches":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.FindInBatches, protoDescriptors, ddlStatements...)
	case "batch_dml":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.BatchDml, protoDescriptors, ddlStatements...)
	case "auto_save_associations":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.AutoSaveAssociations, protoDescriptors, ddlStatements...)
	case "interleaved_tables":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.InterleavedTables, protoDescriptors, ddlStatements...)
	case "read_only_transaction":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.ReadOnlyTransaction, protoDescriptors, ddlStatements...)
	case "read_write_transaction":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.ReadWriteTransaction, protoDescriptors, ddlStatements...)
	case "aborted_transaction":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.AbortedTransaction, protoDescriptors, ddlStatements...)
	case "migrations":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.Migrations)
	case "client_library":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.ClientLibrary, protoDescriptors, ddlStatements...)
	case "uuid_primary_key":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.UuidPrimaryKey)
	case "bit_reversed_sequence":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.BitReversedSequence)
	case "array_data_type":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.ArrayDataType, protoDescriptors, ddlStatements...)
	case "custom_spanner_config":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.CustomSpannerConfig)
	case "protobuf_columns":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.ProtobufColumns, protoDescriptors, ddlStatements...)
	case "data_types":
		emulator.RunSampleOnEmulator(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.DataTypes)
	case "last_statement":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.LastStatement, protoDescriptors, ddlStatements...)
	case "isolation_level":
		emulator.RunSampleOnEmulatorWithDdl(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, snippets.IsolationLevel, protoDescriptors, ddlStatements...)
	default:
		fmt.Printf("unknown sample: %s\n", sample)
		os.Exit(1)
	}
}
