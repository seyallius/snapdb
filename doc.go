// Package dbtestkit. doc.go - Provides a generic, ORM-agnostic test container toolkit
// for integration tests that need a real database (MySQL, PostgreSQL, or SQLite).
//
// dbtestkit is designed to be wired into a Go test package's TestMain function. It boots
// a database (in Docker for MySQL/PostgreSQL, or on local disk for SQLite), restores a
// pre-baked pristine SQL dump for millisecond resets between tests, and exposes a single
// Reset entry-point that returns the database to a known-good state before every test.
//
// Configuration is entirely functional-options based — no environment variables. Every
// project-specific concern (schema creation, data seeding, engine construction, cache
// invalidation) is supplied by the caller via callbacks, so the library never imports
// your ORM or your domain objects.
package snapdb
