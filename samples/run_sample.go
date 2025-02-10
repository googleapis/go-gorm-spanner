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
		emulator.RunSampleOnEmulator(snippets.HelloWorld, ddlStatements...)
	case "insert_data":
		emulator.RunSampleOnEmulator(snippets.InsertData, ddlStatements...)
	case "upsert":
		emulator.RunSampleOnEmulator(snippets.Upsert, ddlStatements...)
	case "batch_insert":
		emulator.RunSampleOnEmulator(snippets.CreateInBatches, ddlStatements...)
	case "find_in_batches":
		emulator.RunSampleOnEmulator(snippets.FindInBatches, ddlStatements...)
	case "batch_dml":
		emulator.RunSampleOnEmulator(snippets.BatchDml, ddlStatements...)
	case "auto_save_associations":
		emulator.RunSampleOnEmulator(snippets.AutoSaveAssociations, ddlStatements...)
	case "interleaved_tables":
		emulator.RunSampleOnEmulator(snippets.InterleavedTables, ddlStatements...)
	case "read_only_transaction":
		emulator.RunSampleOnEmulator(snippets.ReadOnlyTransaction, ddlStatements...)
	case "read_write_transaction":
		emulator.RunSampleOnEmulator(snippets.ReadWriteTransaction, ddlStatements...)
	case "aborted_transaction":
		emulator.RunSampleOnEmulator(snippets.AbortedTransaction, ddlStatements...)
	case "migrations":
		emulator.RunSampleOnEmulator(snippets.Migrations)
	case "client_library":
		emulator.RunSampleOnEmulator(snippets.ClientLibrary, ddlStatements...)
	case "uuid_primary_key":
		emulator.RunSampleOnEmulator(snippets.UuidPrimaryKey)
	case "bit_reversed_sequence":
		emulator.RunSampleOnEmulator(snippets.BitReversedSequence)
	case "array_data_type":
		emulator.RunSampleOnEmulator(snippets.ArrayDataType, ddlStatements...)
	default:
		fmt.Printf("unknown sample: %s\n", sample)
		os.Exit(1)
	}
}
