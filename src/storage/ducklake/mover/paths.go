package mover

import (
	"path/filepath"
	"strings"
)

func isS3Path(p string) bool {
	return strings.HasPrefix(p, "s3://") || strings.HasPrefix(p, "s3a://")
}

// joinLake appends path elements to a lake data root. s3:// bases use URL-style
// '/' joining — filepath.Join on Unix collapses "s3://" to "s3:/".
func joinLake(base string, elems ...string) string {
	if isS3Path(base) {
		out := strings.TrimRight(base, "/")
		for _, e := range elems {
			e = strings.Trim(e, "/")
			if e != "" {
				out += "/" + e
			}
		}
		return out
	}
	return filepath.Join(append([]string{base}, elems...)...)
}

// fileAbs is the on-disk / object path of a catalog file entry.
func fileAbs(dataPath, tablePath, relPath string, isRel int64) string {
	if isRel == 0 {
		return relPath
	}
	return joinLake(dataPath, "main", tablePath, filepath.ToSlash(relPath))
}

// destRelPath is the hive-relative path to use on the destination so
// ducklake_add_data_files can infer the partition from the directory name.
func destRelPath(srcRel, srcAbs, srcData, tablePath string, isRel int64) string {
	if isRel != 0 && strings.TrimSpace(srcRel) != "" {
		return filepath.ToSlash(srcRel)
	}
	prefix := joinLake(srcData, "main", tablePath)
	trimmed := strings.TrimPrefix(strings.ReplaceAll(srcAbs, "\\", "/"), strings.ReplaceAll(prefix, "\\", "/")+"/")
	if trimmed != "" && trimmed != srcAbs {
		return trimmed
	}
	return filepath.ToSlash(filepath.Base(srcAbs))
}

func destAbs(dstData, tablePath, rel string) string {
	return joinLake(dstData, "main", tablePath, filepath.ToSlash(rel))
}

// splitS3URL returns bucket and key for s3://bucket/key or s3a://bucket/key.
func splitS3URL(u string) (bucket, key string, ok bool) {
	u = strings.TrimSpace(u)
	rest := ""
	switch {
	case strings.HasPrefix(u, "s3://"):
		rest = u[len("s3://"):]
	case strings.HasPrefix(u, "s3a://"):
		rest = u[len("s3a://"):]
	default:
		return "", "", false
	}
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		if rest == "" {
			return "", "", false
		}
		return rest, "", true
	}
	return rest[:slash], strings.TrimPrefix(rest[slash+1:], "/"), true
}
