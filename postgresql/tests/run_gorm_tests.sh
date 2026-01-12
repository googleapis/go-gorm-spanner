#!/bin/bash -e

# The following is intentionally commented out, and can be used when running
# the tests locally.

#rm -rf gorm
#git clone git@github.com:go-gorm/gorm.git

# Skip tests that are not supported by Spanner.
# A large part of the tests that are skipped are due to the fact that
# the standard gorm test suite assumes that query results are returned
# in primary key order, and that primary key values are sequential.
#
# Some tests are also skipped due to lack of support in Spanner, such
# as tests that require support for Savepoints. This will be added in
# the future to the Spanner database/sql driver.
skips="\
TestFullSaveAssociations|\
TestHasManyAssociation|\
TestSingleTableHasManyAssociation|\
TestPolymorphicHasManyAssociation|\
TestDuplicateMany2ManyAssociation|\
TestMany2ManyDuplicateBelongsToAssociation|\
TestForeignKeyConstraints|\
TestGenericsPreloads|\
TestGenericsNestedPreloads|\
TestReturningWithNullToZeroValues|\
TestAutoMigrateNullable|\
TestMigrateWithColumnComment|\
TestMigrateIndexes|\
TestMigrateColumns|\
TestMigrateColumnOrder|\
TestMigrateAutoIncrement|\
TestDifferentTypeWithoutDeclaredLength|\
TestMigrateWithDefaultValue|\
TestMigrateIgnoreRelations|\
TestMigrateWithUniqueIndexAndUnique|\
TestManyToManyWithMultiPrimaryKeys|\
TestManyToManyWithCustomizedForeignKeys|\
TestManyToManyWithCustomizedForeignKeys2|\
TestAlterColumnDataType|\
TestNestedPreload2|\
TestNestedPreload3|\
TestNestedPreload4|\
TestNestedPreload6|\
TestManyToManyPreloadWithMultiPrimaryKeys|\
TestManyToManyPreloadForNestedPointer|\
TestNestedManyToManyPreload|\
TestManyToManyPreloadForPointer|\
TestPreloadWithConds|\
TestNestedPreloadWithConds|\
TestNestedPreloadWithNestedJoin|\
TestFind/First/ID#01|\
TestFind/Last/ID#01|\
TestFindInBatchesWithOffsetLimit/Model/ID|\
TestQueryResetNullValue|\
TestTransactionWithSavePoint|\
TestNestedTransactionWithBlock|\
TestDeeplyNestedTransactionWithBlockAndWrappedCallback|\
TestTransactionWithHooks|\
TestUpdateMany2ManyAssociations|\
TestSelectWithUpdate|\
TestSelectWithUpdateWithMap|\
TestSaveWithHooks"

cd gorm/tests || exit 1
rm -f spanner_test.go
cp ../../spanner_test.go ./spanner_test.go

echo "Adding Spanner dialect module"
go get github.com/googleapis/go-gorm-spanner
go mod edit -replace=github.com/googleapis/go-gorm-spanner=../../../../
go mod tidy

if [ "$RUNNER_OS" == "macOS" ]; then
  # Replace the default OpenTestConnection function with one that opens a connection to Spanner.
  sed -i '' -e 's/func OpenTestConnection/func OpenTestConnectionDoNotUse/g' ./tests_test.go
  # Round/truncate to the closest minute instead of second. This is necessary, as Spanner has a higher round-trip
  # latency than local PostgreSQL.
  sed -i '' -e 's/Round(time.Second)/Round(time.Minute)/g' ../utils/tests/utils.go
  sed -i '' -e 's/Truncate(time.Second)/Truncate(time.Minute)/g' ../utils/tests/utils.go
  # Spanner default timestamp handling is different from PostgreSQL.
  sed -i '' -e 's/`gorm:"default:2000-01-02"`/`gorm:"default:'\''2000-01-02 00:00:00Z'\''"`/g' default_value_test.go
  # Add ordering to guarantee which rows are returned.
  sed -i '' -e 's/SELECT name, age FROM users WHERE name = ? or name = ?/SELECT name, age FROM users WHERE name = ? or name = ? order by name/g' sql_builder_test.go
  # Use the PostgreSQL struct for Spanner as well.
  sed -i '' -e 's/DB.Dialector.Name() == "gaussdb"/DB.Dialector.Name() == "gaussdb" || DB.Dialector.Name() == "postgres-spanner"/g' serializer_test.go
else
  # Replace the default OpenTestConnection function with one that opens a connection to Spanner.
  sed -i -e 's/func OpenTestConnection/func OpenTestConnectionDoNotUse/g' ./tests_test.go
  # Round/truncate to the closest minute instead of second. This is necessary, as Spanner has a higher round-trip
  # latency than local PostgreSQL.
  sed -i -e 's/Round(time.Second)/Round(time.Minute)/g' ../utils/tests/utils.go
  sed -i -e 's/Truncate(time.Second)/Truncate(time.Minute)/g' ../utils/tests/utils.go
  # Spanner default timestamp handling is different from PostgreSQL.
  sed -i -e 's/`gorm:"default:2000-01-02"`/`gorm:"default:'\''2000-01-02 00:00:00Z'\''"`/g' default_value_test.go
  # Add ordering to guarantee which rows are returned.
  sed -i -e 's/SELECT name, age FROM users WHERE name = ? or name = ?/SELECT name, age FROM users WHERE name = ? or name = ? order by name/g' sql_builder_test.go
  # Use the PostgreSQL struct for Spanner as well.
  sed -i -e 's/DB.Dialector.Name() == "gaussdb"/DB.Dialector.Name() == "gaussdb" || DB.Dialector.Name() == "postgres-spanner"/g' serializer_test.go
fi

GORM_DIALECT=spannerpg go test -race -count=1 -v \
  -skip "$skips" \
  -failfast \
  ./...
cd ../..

# The following is intentionally commented out, and can be used when running
# the tests locally.

#rm -rf gorm
#git rm gorm -f
