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

### Nested Transactions
`gorm` uses savepoints for nested transactions. Savepoints are currently not supported by Cloud Spanner. Nested
transactions can therefore not be used with GORM.
