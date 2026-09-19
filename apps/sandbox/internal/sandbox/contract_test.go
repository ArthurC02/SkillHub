package sandbox

import "testing"

func TestOnlyReadableInputsAreFetchedIntoTheSandbox(t *testing.T) {
	readable := ObjectGrant{Purpose: "skill_package", Access: "read", URL: "https://store/pkg"}

	for _, tc := range []struct {
		name  string
		grant ObjectGrant
		want  bool
	}{
		{"the skill package", readable, true},
		{"a dataset", ObjectGrant{Purpose: "dataset", Access: "read", URL: "https://store/d"}, true},
		{"a grant the run writes its results through",
			ObjectGrant{Purpose: "dataset", Access: "write", URL: "https://store/d"}, false},
		{"a purpose the driver does not place into the sandbox",
			ObjectGrant{Purpose: "run_output", Access: "read", URL: "https://store/o"}, false},
		{"a readable grant the platform never signed a URL for",
			ObjectGrant{Purpose: "skill_package", Access: "read"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RunRequest{ObjectGrants: []ObjectGrant{tc.grant}}.InputGrants()
			if (len(got) == 1) != tc.want {
				t.Fatalf("InputGrants() = %v, want it carried through: %v", got, tc.want)
			}
		})
	}

	both := RunRequest{ObjectGrants: []ObjectGrant{
		{Purpose: "run_output", Access: "write", URL: "https://store/o"},
		readable,
		{Purpose: "dataset", Access: "read", URL: "https://store/d"},
	}}.InputGrants()
	if len(both) != 2 {
		t.Errorf("InputGrants() kept %d of three grants, want the two readable inputs", len(both))
	}
}
