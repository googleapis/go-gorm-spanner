## Limitations
The following limitations are currently known:

| Limitation             | Workaround                                                                                                                                                                                                                     |
|------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| OnConflict             | OnConflict clauses can only be used with `UpdateAll: true` and `DoNothing: true` clauses. Spanner does not support updating only a subset of the columns. See [upsert.go](../samples/snippets/upsert.go) for a working sample. |
| Nested transactions    | Nested transactions and savepoints are not supported. It is therefore recommended to set the configuration option `DisableNestedTransaction: true,`                                                                            |
| Auto-save associations | Auto-save associations must be used in combination with a `FullSaveAssociations: true` clause. See [auto_save_associations.go](../samples/snippets/auto_save_associations.go) for a working sample.                            |
| Request Priority       | Request priority is not supported.                                                                                                                                                                                             |
| Request Tag            | Request tag is not supported.                                                                                                                                                                                                  |
| Request Options        | Request options are not supported.                                                                                                                                                                                             |
| Partitioned queries    | Partitioned queries are not supported.                                                                                                                                                                                         |
| Backups                | Backups are not supported by this driver. Use the `Cloud Spanner Go client library <https://github.com/googleapis/google-cloud-go/tree/main/spanner>`_ to manage backups programmatically.                                     |
| Manual DDL batching    | `AutoMigrate` and `AutoMigrateDryRun` batch DDL automatically. Calling `SpannerMigrator.StartBatchDDL` directly requires a dedicated connection (for example, wrapping the batch in `db.Connection(func(tx *gorm.DB) error { ... })` or passing a `*sql.Conn` via `Config{Conn: conn}`). |

### Nested Transactions
`gorm` uses savepoints for nested transactions. Savepoints are currently not supported by Cloud Spanner. Nested
transactions can therefore not be used with GORM.

### Manual DDL Batching
`AutoMigrate` and `AutoMigrateDryRun` manage DDL batching automatically on a dedicated connection. If you call
`SpannerMigrator.StartBatchDDL` directly, wrap the batch in `db.Connection(...)` (or configure the dialector with a
`*sql.Conn` via `Config{Conn: conn}`):

```go
err := db.Connection(func(tx *gorm.DB) error {
	m := tx.Migrator().(spannergorm.SpannerMigrator)
	if err := m.StartBatchDDL(); err != nil {
		return err
	}
	defer func() { _ = m.AbortBatch() }()
	if err := m.CreateTable(&Singer{}); err != nil {
		return err
	}
	return m.RunBatch()
})
```
