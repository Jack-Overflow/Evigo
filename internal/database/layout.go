package database

import (
	"context"
	"database/sql"
	"maps"
	"slices"
)

type column struct {
	name       string
	kind       string
	notNull    bool
	primaryKey bool
}

var expectedLayout = map[string][]column{
	"pending": {
		{"device_id", "TEXT", false, true},
		{"attempts", "INTEGER", true, false},
		{"last_attempt_ns", "INTEGER", true, false},
		{"time_left_ns", "INTEGER", true, false},
	},
	"history": {
		{"id", "INTEGER", false, true},
		{"at_ns", "INTEGER", true, false},
		{"kind", "TEXT", true, false},
		{"device_id", "TEXT", true, false},
		{"details", "TEXT", true, false},
	},
}

func layoutMatches(ctx context.Context, sqliteDatabase *sql.DB) bool {
	names, err := schemaObjectNames(ctx, sqliteDatabase)
	if err != nil || !slices.Equal(names, slices.Sorted(maps.Keys(expectedLayout))) {
		return false
	}

	for table, columns := range expectedLayout {
		found, err := tableColumns(ctx, sqliteDatabase, table)
		if err != nil || !slices.Equal(found, columns) {
			return false
		}
	}

	return true
}

func schemaObjectNames(ctx context.Context, sqliteDatabase *sql.DB) ([]string, error) {
	rows, err := sqliteDatabase.QueryContext(ctx, "SELECT name FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}

		names = append(names, name)
	}

	return names, rows.Err()
}

func tableColumns(ctx context.Context, sqliteDatabase *sql.DB, table string) ([]column, error) {
	rows, err := sqliteDatabase.QueryContext(ctx, `SELECT name, type, "notnull", pk FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var columns []column
	for rows.Next() {
		var found column
		var primaryKey int
		if err = rows.Scan(&found.name, &found.kind, &found.notNull, &primaryKey); err != nil {
			return nil, err
		}

		found.primaryKey = primaryKey > 0
		columns = append(columns, found)
	}

	return columns, rows.Err()
}
