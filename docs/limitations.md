## Limitations
The following limitations are currently known:

| Limitation             | Workaround                                                                                                                                                                                                |
|------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| OnConflict             | OnConflict clauses are not supported                                                                                                                                                                      |
| Nested transactions    | Nested transactions and savepoints are not supported. It is therefore recommended to set the configuration option `DisableNestedTransaction: true,`                                                       |
| Locking                | Lock clauses (e.g. `clause.Locking{Strength: "UPDATE"}`) are not supported. These are generally speaking also not required, as the default isolation level that is used by Cloud Spanner is serializable. |
| Auto-save associations | Auto saved associations are not supported, as these will automatically use an OnConflict clause                                                                                                           |
| Session Labelling      | Session labelling is not supported.                                                                                                                                                                       |
| Request Priority       | Request priority is not supported.                                                                                                                                                                        |
| Request Tag            | Request tag is not supported.                                                                                                                                                                             |
| Request Options        | Request options are not supported.                                                                                                                                                                        |
| Partitioned queries    | Partitioned queries are not supported.                                                                                                                                                                    |
| Backups                | Backups are not supported by this driver. Use the `Cloud Spanner Go client library <https://github.com/googleapis/google-cloud-go/tree/main/spanner>`_ to manage backups programmatically.                |

### OnConflict Clauses
`OnConflict` clauses are not supported by Cloud Spanner and should not be used. The following will
therefore not work.

```go
user := User{
    ID:   1,
    Name: "User Name",
}
// OnConflict is not supported and this will return an error.
db.Clauses(clause.OnConflict{DoNothing: true}).Create(&user)
```

### Auto-save Associations
Auto-saving associations will automatically use an `OnConflict` clause in gorm. These are not
supported. Instead, the parent entity of the association must be created before the child entity is
created.

```go
blog := Blog{
    ID:     1,
    Name:   "",
    UserID: 1,
    User: User{
        ID:   1,
        Name: "User Name",
    },
}
// This will fail, as the insert statement for User will use an OnConflict clause.
db.Create(&blog).Error
```

Instead, do the following:

```go
user := User{
    ID:   1,
    Name: "User Name",
    Age:  20,
}
blog := Blog{
    ID:     1,
    Name:   "",
    UserID: 1,
}
db.Create(&user)
db.Create(&blog)
```

### Nested Transactions
`gorm` uses savepoints for nested transactions. Savepoints are currently not supported by Cloud Spanner. Nested
transactions can therefore not be used with GORM.

### Locking
Locking clauses, like `clause.Locking{Strength: "UPDATE"}`, are not supported. These are generally speaking also not
required, as Cloud Spanner uses isolation level `serializable` for read/write transactions.
