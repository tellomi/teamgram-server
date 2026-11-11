package mysqlpool

import (
	"context"
	"database/sql"
	"unsafe"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

// ClampSQLX caps the go-zero pool hidden inside a marmota *sqlx.DB.
// go-zero otherwise sets max open to 64 at the moment the pool is created.
// The cap is installed before that pool is handed back to the query.
func ClampSQLX(db *sqlx.DB) {
	if db == nil {
		return
	}
	settings := Resolve(ProcessName(), KindGoZero)
	wrapProvider(unsafe.Pointer(db), func(orig func(context.Context) (*sql.DB, error)) func(context.Context) (*sql.DB, error) {
		return func(ctx context.Context) (*sql.DB, error) {
			sqlDB, err := orig(ctx)
			if err != nil {
				return nil, err
			}
			Apply(sqlDB, settings)
			return sqlDB, nil
		}
	})
}

// iface is the in-memory shape of an interface value.
type iface struct {
	tab  unsafe.Pointer
	data unsafe.Pointer
}

// wrapProvider replaces the connProv on the write connection of a marmota
// DB. The first field of DB is *conn, and the first field of conn is the
// go-zero SqlConn interface. The concrete value is *commonSqlConn, whose
// first field is the provider function.
func wrapProvider(db unsafe.Pointer, wrap func(func(context.Context) (*sql.DB, error)) func(context.Context) (*sql.DB, error)) {
	if db == nil {
		return
	}
	write := *(*unsafe.Pointer)(db)
	if write == nil {
		return
	}
	raw := (*iface)(write)
	if raw == nil || raw.data == nil {
		return
	}
	provPtr := (*func(context.Context) (*sql.DB, error))(raw.data)
	if provPtr == nil || *provPtr == nil {
		return
	}
	*provPtr = wrap(*provPtr)
}
