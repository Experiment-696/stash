package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type CamSnapshot struct {
	SchemaVersion int                `json:"schema_version"`
	Tables        []CamSnapshotTable `json:"tables"`
}

type CamSnapshotTable struct {
	Name    string          `json:"name"`
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
}

type camSnapshotSpec struct {
	name, columns, order string
	keys                 int
}

const currentCamSnapshotVersion = 94

const legacyCamShowsSnapshotColumns = "id,scene_id,category,site_id,show_date,captured_at,source_url,title_override,notes,external_id,sync_state,created_at,updated_at"

type camSnapshotCompatibilityBoundary struct {
	version   int
	lastTable string
}

// camSnapshotCompatibility is the single version-to-table compatibility
// contract. Append one named boundary per schema version; never slice by count.
var camSnapshotCompatibility = []camSnapshotCompatibilityBoundary{
	{version: 89, lastTable: "cam_sync_changes"},
	{version: 90, lastTable: "cam_show_classification_rule_tags"},
	{version: 91, lastTable: "cam_model_profile_provenance"},
	{version: 92, lastTable: "cam_model_social_profiles"},
	{version: 94, lastTable: "cam_completed_recording_audits"},
}

var camSnapshotSpecs = []camSnapshotSpec{
	{"cam_sites", "id,name,base_url,external_key,icon,enabled,created_at,updated_at", "id", 1},
	{"cam_models", "id,display_name,image,notes,status,performer_id,created_at,updated_at", "id", 1},
	{"cam_shows", legacyCamShowsSnapshotColumns + ",show_type,captured_timezone,captured_precision,duration_override_seconds,duration_override_reason", "id", 1},
	{"cam_show_models", "show_id,model_id,billing_order,participation_role", "show_id,model_id", 2},
	{"cam_model_accounts", "id,model_id,site_id,handle,normalized_handle,profile_url,external_account_id,status,first_seen_at,last_seen_at,valid_from,valid_to,last_synced_at,source,confidence,created_at,updated_at", "id", 1},
	{"cam_model_aliases", "id,model_id,account_id,site_id,alias,normalized_alias,valid_from,valid_to,is_current,source,confidence,last_verified_at,created_at,updated_at", "id", 1},
	{"cam_model_user_state", "user_id,model_id,favorite,rating,updated_at", "user_id,model_id", 2},
	{"cam_sync_changes", "id,provider,external_event_id,entity_type,entity_id,proposed_change_json,status,reviewed_by,reviewed_at,created_at", "id", 1},
	{"cam_show_classification_rules", "id,name,pattern,target,category,enabled,created_at,updated_at", "id", 1},
	{"cam_show_classification_rule_tags", "rule_id,tag_id", "rule_id,tag_id", 2},
	{"cam_model_profile_provenance", "id,model_id,account_id,provider,evidence_key,provider_record_id,source_url,observed_at,payload_json,confidence,review_state,reviewed_by,reviewed_at,created_at,updated_at", "id", 1},
	{"cam_show_sites", "show_id,site_id,created_at", "show_id,site_id", 2},
	{"cam_show_links", "id,show_id,site_id,link_type,url,label,source,created_at,updated_at", "id", 1},
	{"cam_model_social_profiles", "id,model_id,platform,icon,handle,url,status,valid_from,valid_to,source,confidence,provenance,created_at,updated_at", "id", 1},
	{"cam_completed_recording_imports", "id,scene_id,show_id,site_id,model_id,configured_root_id,relative_path_hash,fingerprint_size,fingerprint_mtime_ns,fingerprint_mode,fingerprint_device,fingerprint_inode,parser_version,captured_at,captured_timezone,captured_precision,match_state,outcome,created_at", "id", 1},
	{"cam_completed_recording_audits", "id,actor_user_id,preview_id,candidate_id,relative_path_hash,outcome,review_reason_code,redacted_reason,scene_id,site_id,model_id,created_at", "id", 1},
}

func (s *CamShowStore) ExportSnapshot(ctx context.Context) (*CamSnapshot, error) {
	if _, err := getTx(ctx); err != nil {
		return nil, errors.New("cam snapshot export requires one read transaction")
	}
	ret := &CamSnapshot{SchemaVersion: currentCamSnapshotVersion, Tables: make([]CamSnapshotTable, 0, len(camSnapshotSpecs))}
	for _, spec := range camSnapshotSpecs {
		columns := strings.Split(spec.columns, ",")
		rows, err := dbWrapper.QueryxContext(ctx, "SELECT "+spec.columns+" FROM "+spec.name+" ORDER BY "+spec.order)
		if err != nil {
			return nil, err
		}
		table := CamSnapshotTable{Name: spec.name, Columns: columns}
		for rows.Next() {
			values, err := rows.SliceScan()
			if err != nil {
				rows.Close()
				return nil, err
			}
			for i, value := range values {
				if bytes, ok := value.([]byte); ok {
					values[i] = string(bytes)
				}
			}
			table.Rows = append(table.Rows, values)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		ret.Tables = append(ret.Tables, table)
	}
	return ret, nil
}

func camSnapshotSpecsForVersion(version int) ([]camSnapshotSpec, error) {
	lastTable := ""
	for _, boundary := range camSnapshotCompatibility {
		if boundary.version == version {
			lastTable = boundary.lastTable
			break
		}
	}
	if lastTable == "" {
		return nil, fmt.Errorf("unsupported cam snapshot schema %d", version)
	}
	for index, spec := range camSnapshotSpecs {
		if spec.name == lastTable {
			ret := append([]camSnapshotSpec(nil), camSnapshotSpecs[:index+1]...)
			if version < 92 {
				for i := range ret {
					if ret[i].name == "cam_shows" {
						ret[i].columns = legacyCamShowsSnapshotColumns
					}
				}
			}
			return ret, nil
		}
	}
	return nil, fmt.Errorf("cam snapshot compatibility boundary %d references missing table %s", version, lastTable)
}

func (s *CamShowStore) ImportSnapshot(ctx context.Context, snapshot CamSnapshot) (err error) {
	return s.importSnapshot(ctx, snapshot, dbWrapper.Exec)
}

type camSnapshotExec func(context.Context, string, ...interface{}) (sql.Result, error)

func (s *CamShowStore) importSnapshot(ctx context.Context, snapshot CamSnapshot, exec camSnapshotExec) (err error) {
	specs, compatibilityErr := camSnapshotSpecsForVersion(snapshot.SchemaVersion)
	if compatibilityErr != nil {
		return compatibilityErr
	}
	if _, txErr := getTx(ctx); txErr != nil {
		return errors.New("cam snapshot import requires a write transaction")
	}
	byName := make(map[string]CamSnapshotTable, len(snapshot.Tables))
	for _, table := range snapshot.Tables {
		if _, exists := byName[table.Name]; exists {
			return fmt.Errorf("duplicate snapshot table %s", table.Name)
		}
		byName[table.Name] = table
	}
	if _, err = exec(ctx, "SAVEPOINT cam_snapshot_import"); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if _, e := exec(ctx, "ROLLBACK TO cam_snapshot_import"); e != nil {
				err = errors.Join(err, e)
			}
		}
		if _, e := exec(ctx, "RELEASE cam_snapshot_import"); e != nil {
			err = errors.Join(err, e)
		}
	}()
	for _, spec := range specs {
		table, ok := byName[spec.name]
		if !ok {
			return fmt.Errorf("missing snapshot table %s", spec.name)
		}
		if strings.Join(table.Columns, ",") != spec.columns {
			return fmt.Errorf("invalid columns for %s", spec.name)
		}
		placeholders := strings.TrimRight(strings.Repeat("?,", len(table.Columns)), ",")
		for _, row := range table.Rows {
			if len(row) != len(table.Columns) {
				return fmt.Errorf("invalid row width in %s", spec.name)
			}
			if spec.name == "cam_model_aliases" && row[2] != nil {
				var coherent int
				if e := dbWrapper.Get(ctx, &coherent, `SELECT count(*) FROM cam_model_accounts WHERE id IS ? AND model_id IS ? AND (? IS NULL OR site_id IS ?)`, row[2], row[1], row[3], row[3]); e != nil {
					return e
				}
				if coherent != 1 {
					return errors.New("cam_model_aliases account/model/site mismatch")
				}
			}
			if spec.name == "cam_model_profile_provenance" && row[2] != nil {
				var coherent int
				if e := dbWrapper.Get(ctx, &coherent, `SELECT count(*) FROM cam_model_accounts WHERE id IS ? AND model_id IS ?`, row[2], row[1]); e != nil {
					return e
				}
				if coherent != 1 {
					return errors.New("cam_model_profile_provenance account/model mismatch")
				}
			}
			if _, e := exec(ctx, "INSERT OR IGNORE INTO "+spec.name+"("+spec.columns+") VALUES("+placeholders+")", row...); e != nil {
				return e
			}
			where := make([]string, len(table.Columns))
			args := make([]interface{}, len(row))
			for i := range table.Columns {
				where[i] = table.Columns[i] + " IS ?"
				args[i] = row[i]
			}
			var matching int
			query := "SELECT count(*) FROM " + spec.name + " WHERE " + strings.Join(where, " AND ")
			if e := dbWrapper.Get(ctx, &matching, query, args...); e != nil {
				return e
			}
			if matching != 1 {
				return fmt.Errorf("%s row conflicts with existing data", spec.name)
			}
		}
		delete(byName, spec.name)
	}
	if len(byName) > 0 {
		return errors.New("snapshot contains unknown tables")
	}
	return nil
}
