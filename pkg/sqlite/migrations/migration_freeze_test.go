package migrations

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestFrozenMigrationChecksums(t *testing.T) {
	manifest, err := os.Open("FROZEN_MIGRATIONS.sha256")
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	checked := 0
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			t.Fatalf("invalid frozen migration entry %q", line)
		}
		body, err := os.ReadFile(parts[1])
		if err != nil {
			t.Fatal(err)
		}
		// Canonicalize line endings so Git archives and platform checkouts
		// verify the same migration content without weakening the check.
		body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
		digest := sha256.Sum256(body)
		got := strings.ToUpper(hex.EncodeToString(digest[:]))
		if got != strings.ToUpper(parts[0]) {
			t.Fatalf("frozen migration %s changed: got %s want %s; add a new forward migration instead", parts[1], got, parts[0])
		}
		checked++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if checked < 2 {
		t.Fatalf("checked only %d frozen migrations", checked)
	}
}
