env "local" {
  url = "postgres://pguser:pgpass@localhost:5432/db?sslmode=disable"
  migration {
    dir = "file://migrations"
  }
}
