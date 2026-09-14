# FleetForge SQL migrations

Migration files are immutable, sequential PostgreSQL SQL files applied only by
`cmd/migrate` through the direct-connection, session-lock path. Production
migrations contain no fixtures or seed data. The first schema migration is
introduced by P03-03.
