package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestCamSnapshotV90ClassificationRoundTripRollbackAndIdempotency(t *testing.T) {
	config.InitializeEmpty()
	open := func(name string) *Database {
		db := NewDatabase()
		if err := db.Open(filepath.Join(t.TempDir(), name)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	createTag := func(db *Database) *models.Tag {
		ctx, err := db.Begin(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		tag := &models.Tag{Name: "Snapshot Classification", CreatedAt: now, UpdatedAt: now}
		if err := db.Tag.Create(ctx, &models.CreateTagInput{Tag: tag}); err != nil {
			t.Fatal(err)
		}
		if err := db.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return tag
	}

	source := open("source-v90.sqlite")
	sourceTag := createTag(source)
	write, err := source.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := source.CamShow.CreateClassificationRule(write, "Timestamp capture", `^\d{4}-\d{2}-\d{2} \d{2}-\d{2}-\d{2}\.mp4$`, CamClassificationTargetBasename, "RECORDED", true, []int{sourceTag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Commit(write); err != nil {
		t.Fatal(err)
	}
	read, err := source.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.CamShow.ExportSnapshot(read)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Commit(read); err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != currentCamSnapshotVersion {
		t.Fatalf("schema version=%d", snapshot.SchemaVersion)
	}
	if camSnapshotRowCount(t, snapshot, "cam_show_classification_rules") != 1 || camSnapshotRowCount(t, snapshot, "cam_show_classification_rule_tags") != 1 {
		t.Fatalf("classification tables missing rows: %+v", snapshot.Tables)
	}

	target := open("target-v90.sqlite")
	targetTag := createTag(target)
	if targetTag.ID != sourceTag.ID {
		t.Fatalf("tag IDs differ: source=%d target=%d", sourceTag.ID, targetTag.ID)
	}
	importCtx, err := target.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var invalid CamSnapshot
	if err := json.Unmarshal(encoded, &invalid); err != nil {
		t.Fatal(err)
	}
	for i := range invalid.Tables {
		if invalid.Tables[i].Name == "cam_show_classification_rule_tags" {
			invalid.Tables[i].Rows[0][1] = float64(targetTag.ID + 999)
		}
	}
	if err := target.CamShow.ImportSnapshot(importCtx, invalid); err == nil {
		t.Fatal("invalid classification tag reference imported")
	}
	if rules, err := target.CamShow.ListClassificationRules(importCtx, false); err != nil || len(rules) != 0 {
		t.Fatalf("failed import did not roll back parent rule: rules=%+v err=%v", rules, err)
	}

	if err := target.CamShow.ImportSnapshot(importCtx, *snapshot); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if err := target.CamShow.ImportSnapshot(importCtx, *snapshot); err != nil {
		t.Fatalf("idempotent import: %v", err)
	}
	rules, err := target.CamShow.ListClassificationRules(importCtx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID || !reflect.DeepEqual(rules[0].TagIDs, []int{targetTag.ID}) {
		t.Fatalf("restored rules=%+v", rules)
	}
	restored, err := target.CamShow.ExportSnapshot(importCtx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, restored) {
		t.Fatalf("round trip differs:\nsource=%+v\ntarget=%+v", snapshot, restored)
	}
	if err := target.Commit(importCtx); err != nil {
		t.Fatal(err)
	}
}

func TestCamSnapshotV89ImportRemainsCompatible(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "v89-compat.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	tables := make([]CamSnapshotTable, 0, 8)
	for _, spec := range mustCamSnapshotSpecsForVersion(t, 89) {
		tables = append(tables, CamSnapshotTable{Name: spec.name, Columns: splitSnapshotColumns(spec.columns)})
	}
	if err := db.CamShow.ImportSnapshot(ctx, CamSnapshot{SchemaVersion: 89, Tables: tables}); err != nil {
		t.Fatalf("migration-89 snapshot import: %v", err)
	}
	if err := db.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCamSnapshotV90ImportRemainsCompatible(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "v90-compat.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	tables := make([]CamSnapshotTable, 0, 10)
	for _, spec := range mustCamSnapshotSpecsForVersion(t, 90) {
		tables = append(tables, CamSnapshotTable{Name: spec.name, Columns: splitSnapshotColumns(spec.columns)})
	}
	if err := db.CamShow.ImportSnapshot(ctx, CamSnapshot{SchemaVersion: 90, Tables: tables}); err != nil {
		t.Fatalf("migration-90 snapshot import: %v", err)
	}
	if err := db.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func splitSnapshotColumns(columns string) []string {
	var ret []string
	start := 0
	for i := range columns {
		if columns[i] == ',' {
			ret = append(ret, columns[start:i])
			start = i + 1
		}
	}
	return append(ret, columns[start:])
}

func TestCamSnapshotCompatibilityContract(t *testing.T) {
	wantLast := map[int]string{
		89: "cam_sync_changes",
		90: "cam_show_classification_rule_tags",
		91: "cam_model_profile_provenance",
		92: "cam_model_social_profiles",
		94: "cam_completed_recording_audits",
		96: "cam_show_user_state",
		97: "cam_show_user_state",
	}
	if len(camSnapshotCompatibility) != len(wantLast) {
		t.Fatalf("compatibility boundaries=%+v", camSnapshotCompatibility)
	}
	for version, lastTable := range wantLast {
		specs := mustCamSnapshotSpecsForVersion(t, version)
		if len(specs) == 0 || specs[len(specs)-1].name != lastTable {
			t.Fatalf("version %d specs=%+v", version, specs)
		}
	}
	if _, err := camSnapshotSpecsForVersion(88); err == nil {
		t.Fatal("unsupported snapshot version accepted")
	}
}

func mustCamSnapshotSpecsForVersion(t *testing.T, version int) []camSnapshotSpec {
	t.Helper()
	specs, err := camSnapshotSpecsForVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	return specs
}
