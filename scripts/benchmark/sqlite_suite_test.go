package main

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteSuiteCorrectnessAndMetrics(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bench.sqlite")
	samples, metrics, err := runSQLiteSuite(dbPath, 25, 5, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 15 {
		t.Fatalf("got %d samples, want 15", len(samples))
	}
	if metrics["error_count"].Value != 0 || metrics["sqlite_rows"].Value != 25 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
	for _, operation := range []string{"insert", "lookup", "upsert"} {
		if metrics["sqlite_"+operation+"_ops_per_second"].Value <= 0 {
			t.Fatalf("missing %s throughput", operation)
		}
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(dbPath)+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var foreignKeys int
	rows, err := db.Query("PRAGMA foreign_key_list(benchmark_records)")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		foreignKeys++
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("got %d foreign keys, want 1", foreignKeys)
	}
	if _, err := db.Exec("INSERT INTO benchmark_records(id,path,digest,owner_id,updated_at) VALUES(999,'/invalid','sha256:invalid',999,0)"); err == nil {
		t.Fatal("invalid owner unexpectedly bypassed the foreign-key constraint")
	}
}
