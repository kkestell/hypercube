# field-sync

The sync service for the Field Notes app. Each device pushes its changes to
`/changes` and pulls the changes it has not seen.

```sh
go run . -addr :9090 -db sync.db
```
