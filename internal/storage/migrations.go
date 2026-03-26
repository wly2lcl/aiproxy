package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func isTableNotExistError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

func RunMigrations(db *sql.DB) error {
	var currentVersion int
	var versionStr string
	err := db.QueryRow(getSchemaVersionQuery).Scan(&versionStr)
	if err != nil {
		if err != sql.ErrNoRows {
			if isTableNotExistError(err) {
				currentVersion = 0
			} else {
				return fmt.Errorf("failed to get schema version: %w", err)
			}
		}
	} else if versionStr != "" {
		currentVersion, err = strconv.Atoi(versionStr)
		if err != nil {
			return fmt.Errorf("invalid schema version %q: %w", versionStr, err)
		}
	}

	files, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".up.sql") {
			migrations = append(migrations, file.Name())
		}
	}
	sort.Strings(migrations)

	for _, migrationFile := range migrations {
		parts := strings.Split(migrationFile, "_")
		if len(parts) < 1 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		if version <= currentVersion {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + migrationFile)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", migrationFile, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", migrationFile, err)
		}

		_, err = tx.Exec(string(content))
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", migrationFile, err)
		}

		_, err = tx.Exec(setSchemaVersionQuery, strconv.Itoa(version))
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update schema version to %d: %w", version, err)
		}

		if err = tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", migrationFile, err)
		}
	}

	return nil
}
