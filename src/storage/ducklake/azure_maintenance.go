// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sipcapture/homer-core/src/storage/ducklake/mover"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// azureObjectStore lists and deletes az:// blobs. The production
// implementation is mover.AzureStore (cached Azure SDK client). Tests inject
// a fake so catalog matching can be verified without a storage account.
type azureObjectStore interface {
	DeleteBlob(ctx context.Context, azURL string) error
	ListDataFiles(ctx context.Context, dataPath string) ([]string, error)
}

// usesAzureNativeFileCleanup reports whether DuckLake file cleanup must bypass
// duckdb-azure. The azure extension rebuilds ChainedTokenCredential on every
// file open, so CALL ducklake_delete_orphaned_files / cleanup_old_files
// against a managed-identity volume storms the IMDS endpoint (20 req/s) and
// fails outright — sipcapture/homer#1023, duckdb/duckdb-azure#171.
// Static account_key / connection_string secrets do not hit IMDS; those
// volumes keep the DuckDB CALL path.
func usesAzureNativeFileCleanup(vol *Volume) bool {
	if vol == nil || vol.Type != VolumeTypeAzure {
		return false
	}
	return UsesAzureCredentialChain(vol.AzureAccountKey, vol.AzureConnectionString)
}

func volumeAzureConfig(vol *Volume) mover.AzureConfig {
	if vol == nil {
		return mover.AzureConfig{}
	}
	return mover.AzureConfig{
		AccountName:      vol.AzureAccountName,
		AccountKey:       vol.AzureAccountKey,
		ConnectionString: vol.AzureConnectionString,
		Endpoint:         vol.AzureEndpoint,
	}
}

// RunAzureNativeFileCleanup physically deletes DuckLake-scheduled and
// orphaned parquet objects on Azure using a cached SDK client, then updates
// the catalog. It is the credential_chain substitute for:
//
//	CALL ducklake_cleanup_old_files(..., cleanup_all => true)
//	CALL ducklake_delete_orphaned_files(..., cleanup_all => true)
func RunAzureNativeFileCleanup(ctx context.Context, db *sql.DB, lakeName, dataPath string, cfg mover.AzureConfig) error {
	store, err := mover.NewAzureStore(cfg)
	if err != nil {
		return err
	}
	return runAzureNativeFileCleanup(ctx, db, lakeName, dataPath, store)
}

func runAzureNativeFileCleanup(ctx context.Context, db *sql.DB, lakeName, dataPath string, store azureObjectStore) error {
	if db == nil {
		return fmt.Errorf("azure native cleanup: database handle is nil")
	}
	if strings.TrimSpace(lakeName) == "" {
		return fmt.Errorf("azure native cleanup: lake name is empty")
	}
	if !isAzurePath(dataPath) {
		return fmt.Errorf("azure native cleanup: data path is not az:// (%s)", dataPath)
	}
	if store == nil {
		return fmt.Errorf("azure native cleanup: object store is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	oldN, err := azureCleanupOldFiles(ctx, db, lakeName, dataPath, store)
	if err != nil {
		return err
	}
	orphanN, err := azureDeleteOrphanedFiles(ctx, db, lakeName, dataPath, store)
	if err != nil {
		return err
	}
	logger.Info("TieredStorageManager: Azure native file cleanup completed",
		"lake", lakeName,
		"old_files_deleted", oldN,
		"orphans_deleted", orphanN)
	return nil
}

type scheduledAzureFile struct {
	id   int64
	path string
}

func azureCleanupOldFiles(ctx context.Context, db *sql.DB, lakeName, dataPath string, store azureObjectStore) (int, error) {
	files, err := queryScheduledAzureFiles(db, lakeName, dataPath)
	if err != nil {
		return 0, err
	}
	var deletedIDs []int64
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return len(deletedIDs), err
		}
		if err := store.DeleteBlob(ctx, f.path); err != nil {
			logger.Warn("Azure native cleanup: failed to delete scheduled file",
				"path", f.path, "error", err)
			continue
		}
		deletedIDs = append(deletedIDs, f.id)
	}
	if err := removeScheduledAzureFiles(db, lakeName, deletedIDs); err != nil {
		return len(deletedIDs), err
	}
	return len(deletedIDs), nil
}

func azureDeleteOrphanedFiles(ctx context.Context, db *sql.DB, lakeName, dataPath string, store azureObjectStore) (int, error) {
	known, err := queryKnownAzureFiles(db, lakeName, dataPath)
	if err != nil {
		return 0, err
	}
	listed, err := store.ListDataFiles(ctx, dataPath)
	if err != nil {
		return 0, err
	}
	knownSet := make(map[string]struct{}, len(known))
	for _, p := range known {
		if id, ok := azureObjectID(p); ok {
			knownSet[id] = struct{}{}
		}
	}
	deleted := 0
	for _, p := range listed {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}
		id, ok := azureObjectID(p)
		if !ok {
			continue
		}
		if _, keep := knownSet[id]; keep {
			continue
		}
		if err := store.DeleteBlob(ctx, p); err != nil {
			logger.Warn("Azure native cleanup: failed to delete orphan",
				"path", p, "error", err)
			continue
		}
		deleted++
	}
	return deleted, nil
}

func azureBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	case int32:
		return x != 0
	default:
		return false
	}
}

func ducklakeMetadataIdent(lakeName string) string {
	return `"` + strings.ReplaceAll("__ducklake_metadata_"+lakeName, `"`, `""`) + `"`
}

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// duckLakeDataPathLiteral is DATA_PATH as DuckLake concatenates it: the
// configured path with a trailing slash, so `{DATA_PATH} || relative` does
// not glue `.../lake` and `main/...` into `.../lakemain/...`.
func duckLakeDataPathLiteral(dataPath string) string {
	p := strings.ReplaceAll(strings.TrimSpace(dataPath), `\`, `/`)
	if p != "" && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

func azureConcatPath(dataPath, rel string, isRelative bool) string {
	rel = strings.ReplaceAll(rel, `\`, `/`)
	if !isRelative {
		return rel
	}
	return duckLakeDataPathLiteral(dataPath) + rel
}

// azureObjectID is a stable container\0key identity so az:// and azure://
// catalog paths compare equal to listed blobs.
func azureObjectID(p string) (string, bool) {
	container, key, ok := mover.SplitAzureURL(p)
	if !ok || container == "" {
		return "", false
	}
	return strings.ToLower(container) + "\x00" + key, true
}

func queryScheduledAzureFiles(db *sql.DB, lakeName, dataPath string) ([]scheduledAzureFile, error) {
	md := ducklakeMetadataIdent(lakeName)
	q := fmt.Sprintf(`SELECT data_file_id, path, path_is_relative
		FROM %s.ducklake_files_scheduled_for_deletion`, md)
	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("list files scheduled for deletion: %w", err)
	}
	defer rows.Close()

	var out []scheduledAzureFile
	for rows.Next() {
		var id int64
		var path string
		var rel any
		if err := rows.Scan(&id, &path, &rel); err != nil {
			return nil, err
		}
		out = append(out, scheduledAzureFile{id: id, path: azureConcatPath(dataPath, path, azureBool(rel))})
	}
	return out, rows.Err()
}

func removeScheduledAzureFiles(db *sql.DB, lakeName string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	md := ducklakeMetadataIdent(lakeName)
	const batch = 200
	for i := 0; i < len(ids); i += batch {
		end := i + batch
		if end > len(ids) {
			end = len(ids)
		}
		var b strings.Builder
		for j, id := range ids[i:end] {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "%d", id)
		}
		q := fmt.Sprintf(`DELETE FROM %s.ducklake_files_scheduled_for_deletion WHERE data_file_id IN (%s)`, md, b.String())
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("remove scheduled cleanup files: %w", err)
		}
	}
	return nil
}

func queryKnownAzureFiles(db *sql.DB, lakeName, dataPath string) ([]string, error) {
	md := ducklakeMetadataIdent(lakeName)
	dp := quoteSQLString(duckLakeDataPathLiteral(dataPath))
	// Mirrors DuckLakeMetadataManager::GetKnownFilesForCleanupQuery:
	// live data/delete files plus anything already scheduled for deletion
	// (those are owned by cleanup_old_files, not orphan delete).
	q := fmt.Sprintf(`
SELECT REPLACE(
	CASE
		WHEN NOT file_relative THEN file_path
		ELSE CASE
			WHEN NOT table_relative THEN table_path || file_path
			ELSE CASE
				WHEN NOT schema_relative THEN schema_path || table_path || file_path
				ELSE %s || schema_path || table_path || file_path
			END
		END
	END,
	'\', '/'
) AS full_path
FROM (
	SELECT s.path AS schema_path, t.path AS table_path, file_path,
		s.path_is_relative AS schema_relative, t.path_is_relative AS table_relative, file_relative
	FROM (
		SELECT f.path AS file_path, f.path_is_relative AS file_relative, table_id
		FROM %s.ducklake_data_file f
		UNION ALL
		SELECT f.path AS file_path, f.path_is_relative AS file_relative, table_id
		FROM %s.ducklake_delete_file f
	) AS f
	JOIN %s.ducklake_table t ON f.table_id = t.table_id
	JOIN %s.ducklake_schema s ON t.schema_id = s.schema_id
) AS r
UNION ALL
SELECT REPLACE(
	CASE WHEN NOT f.path_is_relative THEN f.path ELSE %s || f.path END,
	'\', '/'
)
FROM %s.ducklake_files_scheduled_for_deletion f
`, dp, md, md, md, md, dp, md)

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("list known lake files: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
