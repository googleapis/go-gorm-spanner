# PostgreSQL

This directory contains the Spanner PostgreSQL `gorm` dialect. This dialect is an extension of the standard `gorm`
PostgreSQL dialect. The dialect uses the Spanner `database/sql` driver to connect to Spanner. You do not need
[PGAdapter](https://github.com/GoogleCloudPlatform/pgadapter) to use this dialect.

## Connecting

Call the `New` function in the `github.com/googleapis/go-gorm-spanner/postgresql` package to create a `gorm`
connection to a Spanner PostgreSQL database. Specify a Spanner database name as the connection string, e.g.
`projects/my-project/instances/my-instance/databases/my-database`.

``` go
import (
	"fmt"

	spannerpg "github.com/googleapis/go-gorm-spanner/postgresql"
	_ "github.com/googleapis/go-sql-spanner"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func helloWorld(projectId, instanceId, databaseId string) error {
	db, err := gorm.Open(spannerpg.New(postgres.Config{
		DSN: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v\n", err)
	}
	var msg string
	if err := db.Raw("SELECT $1::varchar as greeting", "Hello World from Spanner PostgreSQL!").Scan(&msg).Error; err != nil {
		return fmt.Errorf("failed to execute query: %v", err)
	}
	fmt.Println(msg)

	return nil
}
```

## Samples

See the [samples directory](samples/snippets) for a list of ready-to-run samples that show how to use Spanner PostgreSQL
features with gorm.

## Migrations

The Spanner PostgreSQL gorm dialect uses a custom migrator that overrides some of the defaults in the standard
PostgreSQL gorm migrator, as Spanner does not support the full DDL dialect of PostgreSQL. This migrator can be used
for development purposes.

It is recommended to use a schema management tool for changes to test and production databases. This ensures that you
have full control over the schema changes that are made, and makes it easier to use
[Spanner best-practices for schema](https://docs.cloud.google.com/spanner/docs/schema-updates#best-practices)
management, such as grouping as many DDL statements into one batch as possible.

Supported schema management tools include:
- golang-migrate: https://github.com/golang-migrate/migrate
- Liquibase: https://github.com/cloudspannerecosystem/liquibase-spanner

