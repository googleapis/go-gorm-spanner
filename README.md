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

#### Advanced Properties
- minSessions (int): Sets the minimum number of sessions in the backing session pool. Defaults to 100.
- maxSessions (int): Sets the maximum number of sessions in the backing session pool. Defaults to 400.
- numChannels (int): Sets the number of gRPC channels to use. Defaults to 4.
- retryAbortsInternally (boolean): Boolean that indicates whether the connection should automatically retry aborted errors. The default is true.
- disableRouteToLeader (boolean): Boolean that indicates if all the requests of type read-write and PDML need to be routed to the leader region. The default is false.
- usePlainText (boolean): : Boolean that indicates whether the connection should use plain text communication or not. Set this to true to connect to local mock servers that do not use SSL. Example: `projects/test-project/instances/test-instance/databases/test-db;usePlainText=true`

Example: `projects/my-project/instances/my-instance/databases/my-db;minSessions=100;maxSessions=400;numChannels=4;retryAbortsInternally=true;disableRouteToLeader=false;usePlainText=false`

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

- Go 1.21
- Go 1.20
- Go 1.19

## Data Types
Cloud Spanner supports the following data types in combination with `gorm`.

| Cloud Spanner Type       | gorm / go type               |
|--------------------------|------------------------------|
| bool                     | bool, sql.NullBool           |
| int64                    | uint, int64, sql.NullInt64   |
| string                   | string, sql.NullString       |
| json                     | spanner.NullJSON             |
| float64                  | float64, sql.NullFloat64     |
| float32                  | float32, spanner.NullFloat32 |
| numeric                  | decimal.NullDecimal          |
| timestamp with time zone | time.Time, sql.NullTime      |
| date                     | datatypes.Date               |
| bytes                    | []byte                       |


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
The Cloud Spanner `gorm` dialect has the following known limitations:

| Limitation                                                                                     | Workaround                                                                                                                                                                                                                     |
|------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Nested transactions                                                                            | Nested transactions and savepoints are not supported. It is therefore recommended to set the configuration option `DisableNestedTransaction: true,`                                                                            |
| Locking                                                                                        | Lock clauses (e.g. `clause.Locking{Strength: "UPDATE"}`) are not supported. These are generally speaking also not required, as the default isolation level that is used by Cloud Spanner is serializable.                      |
| [gorm.Automigrate](https://gorm.io/docs/migration.html#Auto-Migration) with interleaved tables | [Interleaved tables](samples/sample_application) are supported by the Cloud Spanner `gorm` dialect, but Auto-Migration does not support interleaved tables. It is therefore recommended to create interleaved tables manually. |
| [Cloud Spanner stale reads](https://cloud.google.com/spanner/docs/reads#go)                    | Stale reads are not supported by gorm.                                                                                                                                                                                         |    

For the complete list of the limitations, see the [Cloud Spanner GORM limitations](https://github.com/googleapis/go-gorm-spanner/blob/main/docs/limitations.md).

### Nested Transactions
`gorm` uses savepoints for nested transactions. Savepoints are currently not supported by Cloud Spanner. Nested
transactions can therefore not be used with GORM.

### Locking
Locking clauses, like `clause.Locking{Strength: "UPDATE"}`, are not supported. These are generally speaking also not
required, as Cloud Spanner uses isolation level `serializable` for read/write transactions.

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
