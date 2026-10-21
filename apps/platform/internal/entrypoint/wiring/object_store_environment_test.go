package wiring

import (
	"strings"
	"testing"
)

func TestObjectBackupStoreRefusesToStartWithoutEverySetting(t *testing.T) {
	full := map[string]string{
		"OBJSTORE_BACKUP_ENDPOINT": "backup.example:443", "OBJSTORE_BACKUP_ACCESS_KEY": "key",
		"OBJSTORE_BACKUP_SECRET_KEY": "secret", "OBJSTORE_BACKUP_BUCKET": "skillhub-backup",
	}
	for missing := range full {
		t.Run(missing, func(t *testing.T) {
			for name, value := range full {
				if name == missing {
					value = ""
				}
				t.Setenv(name, value)
			}
			_, err := ObjectBackupStoreFromEnv()
			if err == nil || !strings.Contains(err.Error(), "set "+missing) {
				t.Fatalf("error = %v, want it to name %s", err, missing)
			}
		})
	}
	t.Run("all set", func(t *testing.T) {
		for name, value := range full {
			t.Setenv(name, value)
		}
		store, err := ObjectBackupStoreFromEnv()
		if err != nil || store.Location() != "backup.example:443/skillhub-backup" {
			t.Fatalf("store = %v, err = %v", store, err)
		}
	})
}
