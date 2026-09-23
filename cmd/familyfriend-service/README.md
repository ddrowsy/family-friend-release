# Family Friend control service

The control service owns customer/profile configuration and device/profile
assignments.

Run it with:

```sh
go run ./cmd/familyfriend-service -listen :8080 -db family-friend.db
```

Flags:

- `-listen`: HTTP listen address. Defaults to `:8080`.
- `-db`: SQLite database path. Defaults to `family-friend.db`.
