package store

import (
	"regexp"
	"testing"
)

func TestMySQLMigrationsAvoidDefaultsOnTextLikeTypes(t *testing.T) {
	t.Helper()

	forbiddenDefault := regexp.MustCompile(`(?i)\b(?:tinytext|text|mediumtext|longtext|tinyblob|blob|mediumblob|longblob|json)\b[^,\n;]*\bdefault\b`)

	for i, statement := range mysqlMigrations {
		if forbiddenDefault.MatchString(statement) {
			t.Fatalf("mysql migration %d uses a forbidden default on a text-like type:\n%s", i, statement)
		}
	}
}
