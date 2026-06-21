// Package xormadapter. xormadapter.go - Adapts a *xorm.Engine to the
// dbtestkit.Engine interface. Users of xorm (casdoor, etc.) can pass an
// instance of this adapter to their WithEngineInitializer callback.
//
// Example:
//
//	import (
//	    "github.com/seyallius/snapdb"
//	    "github.com/seyallius/snapdb/xormadapter"
//	    "github.com/xorm-io/xorm"
//
// )
//
//	dbtestkit.Run(m,
//	    dbtestkit.WithDriver(mysql.New()),
//	    dbtestkit.WithEngineInitializer(func(env *dbtestkit.Environment) (dbtestkit.Engine, error) {
//	        eng, err := xorm.NewEngine("mysql", env.DSN())
//	        if err != nil {
//	            return nil, err
//	        }
//	        return xormadapter.New(eng), nil
//	    }),
//	    ...
//	)
package xormadapter

import (
	"database/sql"

	"github.com/seyallius/snapdb"
	"github.com/xorm-io/xorm"
)

// ---------------------------------- Types, Variables & Constants ---------------------------------- //

// Adapter wraps a *xorm.Engine to satisfy dbtestkit.Engine.
type Adapter struct {
	eng *xorm.Engine
}

// compile-time interface check.
var _ dbtestkit.Engine = (*Adapter)(nil)

// ------------------------------------------- Constructor(s) --------------------------------------- //

// New returns an Adapter wrapping the given *xorm.Engine.
//
// The Adapter does NOT take ownership of the engine — the caller is
// responsible for closing it (which dbtestkit does via Engine.Close()).
func New(eng *xorm.Engine) *Adapter {
	return &Adapter{eng: eng}
}

// -------------------------------------------- Public API ------------------------------------------ //

// Engine returns the underlying *xorm.Engine, should the caller need to
// pass it to ORM-specific helpers (e.g. Sync2).
func (a *Adapter) Engine() *xorm.Engine { return a.eng }

// Exec implements dbtestkit.Engine.
func (a *Adapter) Exec(query string, args ...any) (sql.Result, error) {
	return a.eng.Exec(a.prependQuery(query, args)...)
}

// QueryString implements dbtestkit.Engine.
func (a *Adapter) QueryString(query string, args ...any) ([]map[string]string, error) {
	return a.eng.QueryString(a.prependQuery(query, args)...)
}

// Ping implements dbtestkit.Engine.
func (a *Adapter) Ping() error { return a.eng.Ping() }

// Close implements dbtestkit.Engine.
func (a *Adapter) Close() error { return a.eng.Close() }

// ClearCache implements dbtestkit.Engine. Forwards to xorm.Engine.ClearCache,
// which flushes the ORM-level Ristretto store. Safe to call even if caching
// is disabled (ClearCache is a no-op in that case).
func (a *Adapter) ClearCache() error { return a.eng.ClearCache() }

// ------------------------------------------- Internal Helpers ------------------------------------- //

// prependQuery prepends the query string to the args slice.
func (a *Adapter) prependQuery(query string, args []any) []any {
	sqlOrArgs := make([]any, 0, len(args)+1)
	sqlOrArgs = append(sqlOrArgs, query)
	for _, arg := range args {
		sqlOrArgs = append(sqlOrArgs, arg)
	}
	return sqlOrArgs
}
