package main

import (
	"reflect"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/identity"
)

// DDD-018 left this command without a wiring test on the grounds that each
// subcommand is a single struct literal next to its only caller, which a reader
// checks by looking at it. The account purge broke that: its literal now carries
// five injected steps, one per context that owns rows in a workspace (ADR-034),
// and this is the process that actually runs the purge — the API only offers the
// button. Dropping one of them is a compliance hole, so it gets a test.
//
// Reflection rather than five named assertions so that a sixth context added to
// identity.Service is covered here the day it appears, not the day somebody
// remembers this file. No database and no object storage: a nil pool is never
// dialled because nothing here queries.
func TestPurgeServiceCarriesEveryContextsStep(t *testing.T) {
	svc := reflect.ValueOf(*purgeService(nil))
	for i := range svc.NumField() {
		field := svc.Field(i)
		if field.Type() == reflect.TypeFor[identity.WorkspacePurge]() && field.IsNil() {
			t.Errorf("identity.Service.%s is nil: purge-accounts would refuse to run",
				svc.Type().Field(i).Name)
		}
	}
}
