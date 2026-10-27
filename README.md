# lemmego/gormconnector

Wires [GORM](https://gorm.io) into a Lemmego application's lifecycle: it opens
the connection from your configured database block, registers it in the
container, and shuts it down cleanly.

Use it when you want GORM as your data layer. For the Lemmego ORM instead, use
[`ormconnector`](https://github.com/lemmego/ormconnector).

```go
// bootstrap/providers.go
func LoadProviders() []app.Provider {
    return []app.Provider{
        &gormconnector.Provider{UseGPA: true},
        // … everything that needs a database goes after this
    }
}
```

## Install

```bash
go get github.com/lemmego/gormconnector
```

Then import the driver you actually use — the connector does not pick one for
you, so a deployment links only the database it talks to.

## With or without GPA

```go
&gormconnector.Provider{UseGPA: true}   // type-safe repositories
&gormconnector.Provider{UseGPA: false}  // a raw *gorm.DB
```

With `UseGPA: true` you also get a
[`gpagorm`](https://github.com/lemmego/gpagorm) provider registered, so you
can reach a typed repository:

```go
db := gormconnector.Get(c.App())
repo := gpagorm.GetRepository[User](db)

user, err := repo.FindByID(c.RequestContext(), 123)   // *User, not interface{}
```

With `UseGPA: false` you get the `*gorm.DB` itself and write GORM as you
normally would.

## Where the connection comes from

From your application's own configuration, not from this module. That is the
point of a connector: the DSN your migrations run against and the DSN your
handlers query are the same one, so they cannot drift.

Supported drivers: `postgres`, `postgresql`, `mysql`, `sqlite`, `sqlite3`,
`sqlserver`, `mssql`.

To reach past the connector when you need something GORM-specific:

```go
conn := &gormconnector.Provider{UseGPA: true}
conn.WithGORMConfig(&gorm.Config{PrepareStmt: true}).WithGPAConfig(cfg)

return []app.Provider{conn, …}
```

`SQLDB()` gives you the underlying `*sql.DB` for anything that wants the
standard library — a session store, a health check, a migration runner.

## Ordering

List this provider **before** anything that needs a database. A provider that
calls `db.Resolve(a)` with nothing registered yet does not fail loudly; it
quietly derives its own connection, and then you have two pools against one
server and one of them ignoring your configuration.

## Generators

Two commands come with it:

```bash
lemmego gorm:model Post    # a GORM model with gorm: struct tags
lemmego gorm:repo Post     # a repository, with GPA integration
```

## Testing

```bash
go test ./...
GOWORK=off go test ./...   # as a published module
```
