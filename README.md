# go-gorm-spanner

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/googleapis/go-gorm-spanner)

[Google Cloud Spanner](https://cloud.google.com/spanner) ORM for
Go's [GORM](https://gorm.io/) implementation.

``` go
import (
    "gorm.io/gorm"
    _ "github.com/googleapis/go-sql-spanner"

    spannergorm "github.com/googleapis/go-gorm-spanner"
)

db, err := gorm.Open(spannergorm.New(spannergorm.Config{
    DriverName: "spanner",
    DSN:        "projects/PROJECT/instances/INSTANCE/databases/DATABASE",
}), &gorm.Config{PrepareStmt: true})
if err != nil {
    log.Fatal(err)
}

// Print singers with more than 500 likes.
type Singer struct {
    gorm.Model
    Text         string
    Likes        int
}
var singers []Singer
if err := db.Where("likes > ?", 500).Find(&singers).Error; err != nil {
    log.Fatal(err)
}
for s := range singers {
    fmt.Println(s.ID, s.Text)
}
```

### Connection URL Properties

The Cloud Spanner GORM supports the following connection URL properties

#### Commonly Used Properties
- credentials (String): File name for the credentials to use. The connection will use the default credentials of the environment if no credentials file is specified in the connection string. Example: `projects/my-project/instances/my-instance/databases/my-db;credentials=/path/to/credentials.json`
- optimizerVersion (String): Sets the default query optimizer version to use for this connection. See also https://cloud.google.com/spanner/docs/query-optimizer/query-optimizer-versions.
- isolationLevel (String): Sets the default isolation level for read/write transaction. The default is `sql.LevelSerializable`. Other supported values are `sql.LevelRepeatableRead`. Example: `fmt.Sprintf("projects/my-project/instances/my-instance/databases/my-db;isolationLevel=%s", sql.LevelRepeatableRead)`

#### Advanced Properties
- minSessions (int): Sets the minimum number of sessions in the backing session pool. Defaults to 100.
- maxSessions (int): Sets the maximum number of sessions in the backing session pool. Defaults to 400.
- numChannels (int): Sets the number of gRPC channels to use. Defaults to 4.
- retryAbortsInternally (boolean): Boolean that indicates whether the connection should automatically retry aborted errors. The default is true.
- disableRouteToLeader (boolean): Boolean that indicates if all the requests of type read-write and PDML need to be routed to the leader region. The default is false.
- usePlainText (boolean): : Boolean that indicates whether the connection should use plain text communication or not. Set this to true to connect to local mock servers that do not use SSL. Example: `projects/test-project/instances/test-instance/databases/test-db;usePlainText=true`

Example: `projects/my-project/instances/my-instance/databases/my-db;minSessions=100;maxSessions=400;numChannels=4;retryAbortsInternally=true;disableRouteToLeader=false;usePlainText=false`

#### Additional Spanner Configuration
You can also connect `gorm` to Spanner using a `driver.Connector`. This allows you to supply additional configuration
for the Spanner client that should be used for `gorm`:

```go
// Create a function that sets the Spanner client configuration for the database connection.
configureFunction := func(config *spanner.ClientConfig, opts *[]option.ClientOption) {
    // Set a default query optimizer version that the client should use.
    config.QueryOptions = spanner.QueryOptions{Options: &spannerpb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1"}}
}
// Create a ConnectorConfig to use with the Spanner database/sql driver.
config := spannerdriver.ConnectorConfig{
    Project:      projectId,
    Instance:     instanceId,
    Database:     databaseId,
    Configurator: configureFunction,
}
// Create a Connector for Spanner. This Connector instance should be re-used for all gorm connections.
c, err := spannerdriver.CreateConnector(config)
db, err := gorm.Open(
    spannergorm.New(spannergorm.Config{Connector: c}),
    &gorm.Config{PrepareStmt: true})
```

See [custom_spanner_config.go](samples/snippets/custom_spanner_config.go) for a working sample application.

## Emulator

See the [Google Cloud Spanner Emulator](https://cloud.google.com/spanner/docs/emulator) support to learn how to start the emulator.
When the emulator has been started and the environment variable has been set, `gorm` will automatically connect to the emulator
instead of Cloud Spanner.

```
$ gcloud emulators spanner start
$ export SPANNER_EMULATOR_HOST=localhost:9010
```

## [Go Versions Supported](#supported-versions)

Our libraries are compatible with at least the three most recent, major Go
releases. They are currently compatible with:

- Go 1.23
- Go 1.22

## Data Types
Spanner supports the following data types in combination with `gorm`.

| Cloud Spanner Type       | gorm / go type                                    |
|--------------------------|---------------------------------------------------|
| bool                     | bool, sql.NullBool, spanner.NullBool              |
| int64                    | uint, int64, sql.NullInt64, spanner.NullInt64     |
| string                   | string, sql.NullString, spanner.NullString        |
| json                     | spanner.NullJSON                                  |
| float64                  | float64, sql.NullFloat64, spanner.NullFloat64     |
| float32                  | float32, spanner.NullFloat32, spanner.NullFloat32 |
| numeric                  | big.Rat, spanner.NullNumeric                      |
| timestamp with time zone | time.Time, sql.NullTime, spanner.NullTime         |
| date                     | civil.Date, spanner.NullDate                      |
| bytes                    | []byte                                            |

See [data_types.go](samples/snippets/data_types.go) for a working sample for each
data type.

You can also use arrays and protobuf columns with `gorm`. See the following samples
for how to map and use those types:
* Arrays: [array_data_type.go](/samples/snippets/array_data_type.go)
* Protobuf: [protobuf_columns.go](/samples/snippets/protobuf_columns.go)

## Auto-increment Primary Keys
Columns that are marked as auto-increment in `gorm` use `IDENTITY` columns in Spanner
by default. `IDENTITY` columns use a backing bit-reversed sequence for value generation.
These values are guaranteed to be unique and safe to use as generated primary key values
with Spanner. See https://cloud.google.com/spanner/docs/primary-key-default-value#identity-columns
for more information on `IDENTITY` columns.

The following model uses an `IDENTITY` column with a backing bit-reversed sequence for
primary key values:

```go
type singer struct {
	gorm.Model
	Name string
}
```

### Sequence Kind
`IDENTITY` columns by default use a bit-reversed sequence to generate values. You can set
the type of sequence to use in the `gorm` dialector configuration. The default and currently
only supported value is `BIT_REVERSED_POSITIVE`.

In addition, the configuration option supports two special values:
- `DISABLED`: Disable the use of `IDENTITY` columns and use bit-reversed sequences instead. See below for more information.
- `AUTO_INCREMENT`: Generate `AUTO_INCREMENT` columns instead of  `IDENTITY` columns. `AUTO_INCREMENT` columns use
  the default sequence kind that has been configured in the database. See https://cloud.google.com/spanner/docs/primary-key-default-value#serial-auto-increment
  for more information on `AUTO_INCREMENT` columns.

Example:

```go
// This dialector uses AUTO_INCREMENT columns instead of IDENTITY columns.
dialector := New(Config{
    DriverName:          "spanner",
    DSN:                 fmt.Sprintf("projects/my-project/instances/my-instance/databases/my-database"),
    DefaultSequenceKind: "AUTO_INCREMENT",
})
```

### Sequences
You can also manually assign a sequence to be used for primary key generation for a
model. Use the `gorm_sequence_name` tag to specify a sequence name.

This model uses a sequence named `singer_sequence` for primary key generation.

```go
type singer struct {
	ID       uint `gorm:"primarykey" gorm_sequence_name:"singer_sequence"`
	Name     string
}
```

You can also configure Spanner `gorm` to use sequences for to generate primary key
values for all models. Set the `DefaultSequenceKind` in the dialector configuration
for this:

```go
dialector := New(Config{
    DriverName:          "spanner",
    DSN:                 fmt.Sprintf("projects/my-project/instances/my-instance/databases/my-database"),
    DefaultSequenceKind: "DISABLED",
})

// This model uses a bit-reversed sequence for primary key generation,
// as DefaultSequenceKind has been set to DISABLED in the configuration.
type singer struct {
    gorm.Model
    Name string
}
```

## AutoMigrate Dry Run
The Spanner `gorm` dialect supports dry-runs for auto-migration. Use this to get the
DDL statements that would be generated and executed by auto-migration. You can manually
verify and modify these statements to optimize your data model.

Example:

```go
tables := []interface{}{&singer{}, &album{}}

// Unwrap the underlying SpannerMigrator interface. This interface supports
// the `AutoMigrateDryRun` method, which does not actually execute the
// generated statements, and instead just returns these as an array.
m := db.Migrator()
migrator, ok := m.(spannergorm.SpannerMigrator)
if !ok {
    return fmt.Errorf("unexpected migrator type: %v", m)
}
statements, err := migrator.AutoMigrateDryRun(tables...)
```

## Limitations
The Spanner `gorm` dialect has the following known limitations:

| Limitation                                                                                     | Workaround                                                                                                                                                                                                               |
|------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Nested transactions                                                                            | Nested transactions and savepoints are not supported. It is therefore recommended to set the configuration option `DisableNestedTransaction: true,`                                                                      |
| [gorm.Automigrate](https://gorm.io/docs/migration.html#Auto-Migration) with interleaved tables | [Interleaved tables](samples/sample_application) are supported by the Spanner `gorm` dialect, but Auto-Migration does not support interleaved tables. It is therefore recommended to create interleaved tables manually. |
| [Spanner stale reads](https://cloud.google.com/spanner/docs/reads#go)                          | Stale reads are not supported by gorm.                                                                                                                                                                                   |

For the complete list of the limitations, see the [Spanner GORM limitations](/docs/limitations.md).

### Nested Transactions
`gorm` uses savepoints for nested transactions. Savepoints are currently not supported by Cloud Spanner. Nested
transactions can therefore not be used with GORM.

## Authorization

By default, each API will use [Google Application Default Credentials](https://developers.google.com/identity/protocols/application-default-credentials)
for authorization credentials used in calling the API endpoints. This will allow your
application to run in many environments without requiring explicit configuration.

## Contributing

Contributions are welcome. Please, see the
[CONTRIBUTING](https://github.com/googleapis/go-gorm-spanner/blob/main/CONTRIBUTING.md)
document for details.

Please note that this project is released with a Contributor Code of Conduct.
By participating in this project you agree to abide by its terms.
See [Contributor Code of Conduct](https://github.com/googleapis/go-gorm-spanner/blob/main/CODE_OF_CONDUCT.md)
for more information.
