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
		emulator.RunSampleOnEmulator(func(project string, instance string, database string) error {
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
		emulator.RunSampleOnEmulatorWithDdl(snippets.HelloWorld, protoDescriptors, ddlStatements...)
	case "insert_data":
		emulator.RunSampleOnEmulatorWithDdl(snippets.InsertData, protoDescriptors, ddlStatements...)
	case "upsert":
		emulator.RunSampleOnEmulatorWithDdl(snippets.Upsert, protoDescriptors, ddlStatements...)
	case "batch_insert":
		emulator.RunSampleOnEmulatorWithDdl(snippets.CreateInBatches, protoDescriptors, ddlStatements...)
	case "find_in_batches":
		emulator.RunSampleOnEmulatorWithDdl(snippets.FindInBatches, protoDescriptors, ddlStatements...)
	case "batch_dml":
		emulator.RunSampleOnEmulatorWithDdl(snippets.BatchDml, protoDescriptors, ddlStatements...)
	case "auto_save_associations":
		emulator.RunSampleOnEmulatorWithDdl(snippets.AutoSaveAssociations, protoDescriptors, ddlStatements...)
	case "interleaved_tables":
		emulator.RunSampleOnEmulatorWithDdl(snippets.InterleavedTables, protoDescriptors, ddlStatements...)
	case "read_only_transaction":
		emulator.RunSampleOnEmulatorWithDdl(snippets.ReadOnlyTransaction, protoDescriptors, ddlStatements...)
	case "read_write_transaction":
		emulator.RunSampleOnEmulatorWithDdl(snippets.ReadWriteTransaction, protoDescriptors, ddlStatements...)
	case "aborted_transaction":
		emulator.RunSampleOnEmulatorWithDdl(snippets.AbortedTransaction, protoDescriptors, ddlStatements...)
	case "migrations":
		emulator.RunSampleOnEmulator(snippets.Migrations)
	case "client_library":
		emulator.RunSampleOnEmulatorWithDdl(snippets.ClientLibrary, protoDescriptors, ddlStatements...)
	case "uuid_primary_key":
		emulator.RunSampleOnEmulator(snippets.UuidPrimaryKey)
	case "bit_reversed_sequence":
		emulator.RunSampleOnEmulator(snippets.BitReversedSequence)
	case "custom_spanner_config":
		emulator.RunSampleOnEmulator(snippets.CustomSpannerConfig)
	case "protobuf_columns":
		emulator.RunSampleOnEmulatorWithDdl(snippets.ProtobufColumns, protoDescriptors, ddlStatements...)
	default:
		fmt.Printf("unknown sample: %s\n", sample)
		os.Exit(1)
	}
}
