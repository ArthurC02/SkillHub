package wiring

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
)

func ObjectStoreFromEnv() (*objstore.Client, error) {
	return objstore.New(
		envx.Or(os.Getenv("OBJSTORE_ENDPOINT"), "localhost:8333"),
		os.Getenv("OBJSTORE_ACCESS_KEY"),
		os.Getenv("OBJSTORE_SECRET_KEY"),
		envx.Or(os.Getenv("OBJSTORE_BUCKET"), "skillhub"),
		os.Getenv("OBJSTORE_SSL") == "1",
	)
}

const (
	ObjectBackupRetention = 35 * 24 * time.Hour
	ObjectBackupSample    = 20
)

var objectBackupSettings = []string{
	"OBJSTORE_BACKUP_ENDPOINT", "OBJSTORE_BACKUP_ACCESS_KEY", "OBJSTORE_BACKUP_SECRET_KEY", "OBJSTORE_BACKUP_BUCKET",
}

func ObjectBackupStoreFromEnv() (*objstore.Client, error) {
	var unset []string
	for _, name := range objectBackupSettings {
		if os.Getenv(name) == "" {
			unset = append(unset, name)
		}
	}
	if len(unset) > 0 {
		return nil, fmt.Errorf("the object backup has nowhere to go: set %s", strings.Join(unset, ", "))
	}
	return objstore.New(
		os.Getenv("OBJSTORE_BACKUP_ENDPOINT"),
		os.Getenv("OBJSTORE_BACKUP_ACCESS_KEY"),
		os.Getenv("OBJSTORE_BACKUP_SECRET_KEY"),
		os.Getenv("OBJSTORE_BACKUP_BUCKET"),
		os.Getenv("OBJSTORE_BACKUP_SSL") == "1",
	)
}
