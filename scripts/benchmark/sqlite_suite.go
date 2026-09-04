package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func removeSQLiteRunFiles(dbPath string) error {
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

type sqliteSample struct {
	Operation string  `json:"operation"`
	Rows      int     `json:"rows"`
	Seconds   float64 `json:"seconds"`
}

func runSQLiteSuite(dbPath string, records, repetitions int, deadline time.Time) ([]sqliteSample, map[string]metric, error) {
	if records <= 0 {
		return nil, nil, errors.New("SQLite record count must be positive")
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(dbPath)+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE benchmark_owners (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL UNIQUE
	);
	CREATE TABLE benchmark_records (
		id INTEGER PRIMARY KEY,
		path TEXT NOT NULL UNIQUE,
		digest TEXT NOT NULL,
		owner_id INTEGER NOT NULL REFERENCES benchmark_owners(id) ON DELETE RESTRICT,
		updated_at INTEGER NOT NULL
	)`); err != nil {
		return nil, nil, err
	}
	ownerTx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	for id := 1; id <= 17; id++ {
		if _, err := ownerTx.Exec("INSERT INTO benchmark_owners(id,name) VALUES(?,?)", id, fmt.Sprintf("owner-%02d", id)); err != nil {
			ownerTx.Rollback()
			return nil, nil, err
		}
	}
	if err := ownerTx.Commit(); err != nil {
		return nil, nil, err
	}

	var samples []sqliteSample
	durations := map[string][]float64{"insert": {}, "lookup": {}, "upsert": {}}
	for repetition := -1; repetition < repetitions; repetition++ {
		if time.Now().After(deadline) {
			return nil, nil, errors.New("SQLite suite exceeded the configured wall-time ceiling")
		}
		if _, err := db.Exec("DELETE FROM benchmark_records"); err != nil {
			return nil, nil, err
		}
		insertSeconds, err := timedInsert(db, records, repetition)
		if err != nil {
			return nil, nil, err
		}
		lookupSeconds, err := timedLookups(db, records)
		if err != nil {
			return nil, nil, err
		}
		upsertSeconds, err := timedUpserts(db, records, repetition)
		if err != nil {
			return nil, nil, err
		}
		if repetition >= 0 {
			values := map[string]float64{"insert": insertSeconds, "lookup": lookupSeconds, "upsert": upsertSeconds}
			for operation, seconds := range values {
				samples = append(samples, sqliteSample{Operation: operation, Rows: records, Seconds: seconds})
				durations[operation] = append(durations[operation], seconds)
			}
		}
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM benchmark_records").Scan(&count); err != nil {
		return nil, nil, err
	}
	if count != records {
		return nil, nil, fmt.Errorf("SQLite correctness gate: got %d rows, want %d", count, records)
	}
	var foreignKeyViolations int
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		foreignKeyViolations++
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	if foreignKeyViolations != 0 {
		return nil, nil, fmt.Errorf("SQLite correctness gate: %d foreign-key violations", foreignKeyViolations)
	}

	metrics := map[string]metric{"error_count": {Value: 0, Unit: "count", LowerIsBetter: true}, "sqlite_rows": {Value: float64(records), Unit: "count", LowerIsBetter: false}}
	for operation, values := range durations {
		stats := calculateStats(values)
		prefix := "sqlite_" + operation + "_"
		metrics[prefix+"median"] = metric{Value: stats.Median, Unit: "seconds", LowerIsBetter: true}
		metrics[prefix+"p95"] = metric{Value: stats.P95, Unit: "seconds", LowerIsBetter: true}
		metrics[prefix+"cv"] = metric{Value: stats.CV, Unit: "ratio", LowerIsBetter: true}
		metrics[prefix+"ops_per_second"] = metric{Value: float64(records) / stats.Median, Unit: "ops/s", LowerIsBetter: false}
	}
	return samples, metrics, nil
}

func timedInsert(db *sql.DB, records, repetition int) (float64, error) {
	started := time.Now()
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare("INSERT INTO benchmark_records(id,path,digest,owner_id,updated_at) VALUES(?,?,?,?,?)")
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	for i := 1; i <= records; i++ {
		if _, err := stmt.Exec(i, fmt.Sprintf("/synthetic/%08d.dat", i), fmt.Sprintf("sha256:%064x", i), i%17+1, repetition+2); err != nil {
			stmt.Close()
			tx.Rollback()
			return 0, err
		}
	}
	if err := stmt.Close(); err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return time.Since(started).Seconds(), nil
}

func timedLookups(db *sql.DB, records int) (float64, error) {
	started := time.Now()
	stmt, err := db.Prepare("SELECT digest FROM benchmark_records WHERE path = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for i := 1; i <= records; i++ {
		var digest string
		if err := stmt.QueryRow(fmt.Sprintf("/synthetic/%08d.dat", i)).Scan(&digest); err != nil {
			return 0, err
		}
		if digest == "" {
			return 0, errors.New("SQLite correctness gate: empty digest")
		}
	}
	return time.Since(started).Seconds(), nil
}

func timedUpserts(db *sql.DB, records, repetition int) (float64, error) {
	started := time.Now()
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare("INSERT INTO benchmark_records(id,path,digest,owner_id,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at")
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	for i := 1; i <= records; i++ {
		if _, err := stmt.Exec(i, fmt.Sprintf("/synthetic/%08d.dat", i), fmt.Sprintf("sha256:%064x", i), i%17+1, repetition+100); err != nil {
			stmt.Close()
			tx.Rollback()
			return 0, err
		}
	}
	if err := stmt.Close(); err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return time.Since(started).Seconds(), nil
}
