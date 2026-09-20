package main

import (
	"strings"
	"testing"
)

func nodeSites(gateway, subnet, port, runtime, preflightRuntime string) map[string]string {
	return map[string]string{
		"infra/deploy/sandbox/daemon.json": `{
  "runtimes": { "` + runtime + `": { "path": "/usr/local/bin/` + runtime + `" } },
  "bip": "` + gateway + `/16"
}
`,
		"infra/deploy/sandbox/bin/skillhub-bootstrap": "#!/bin/sh\n" +
			"bridge_gateway=" + gateway + "\n" +
			"SKILLHUB_SANDBOX_RUNTIME=" + runtime + "\n" +
			"SKILLHUB_SANDBOX_ADDR=$SKILLHUB_PRIVATE_IP:" + port + "\n",
		"infra/deploy/sandbox/bin/skillhub-preflight": "#!/bin/sh\n" +
			"if ! docker info | grep -qw " + preflightRuntime + "; then\n",
		"infra/deploy/sandbox/systemd/skillhub-egress-flows.service": "[Service]\n" +
			"ExecStart=/bin/sh -c 'conntrack --orig-src " + subnet + " | sandboxd egress-record'\n",
		"tools/egress/render.py": "def render():\n" +
			`    a('        iifname $SANDBOX_IFACE tcp dport ` + port + ` counter log prefix "skillhub-drop-sandboxd " drop')` + "\n" +
			`    a("        ip saddr " + control_plane + " tcp dport ` + port + ` counter accept")` + "\n",
	}
}

func TestDeploymentFactsThatAgreeAreAccepted(t *testing.T) {
	t.Parallel()
	root := writeSites(t, nodeSites("172.17.0.1", "172.17.0.0/16", "9000", "runsc", "runsc"))
	if problems := sandboxNodeFactProblems(root); len(problems) != 0 {
		t.Fatalf("a node whose files agree was rejected: %v", problems)
	}
}

func TestEachDeploymentFactIsCaughtWhenOnlyOneSiteChanges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		sites map[string]string
		want  string
	}{
		{"the bridge gateway moved in the bootstrap only",
			nodeSites("172.17.0.1", "172.17.0.0/16", "9000", "runsc", "runsc"), "the docker bridge gateway"},
		{"the bridge subnet moved in the flow recorder only",
			nodeSites("172.17.0.1", "10.0.0.0/8", "9000", "runsc", "runsc"), "the docker bridge subnet"},
		{"the port moved in the firewall only",
			nodeSites("172.17.0.1", "172.17.0.0/16", "9000", "runsc", "runsc"), "the sandboxd listening port"},
		{"the runtime was renamed in preflight only",
			nodeSites("172.17.0.1", "172.17.0.0/16", "9000", "runsc", "runc"), "the container runtime name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sites := tc.sites
			switch tc.want {
			case "the docker bridge gateway":
				sites["infra/deploy/sandbox/bin/skillhub-bootstrap"] = strings.Replace(
					sites["infra/deploy/sandbox/bin/skillhub-bootstrap"],
					"bridge_gateway=172.17.0.1", "bridge_gateway=10.0.0.1", 1)
			case "the sandboxd listening port":
				sites["tools/egress/render.py"] = strings.Replace(
					sites["tools/egress/render.py"], "tcp dport 9000 counter accept",
					"tcp dport 9100 counter accept", 1)
			}
			problems := sandboxNodeFactProblems(writeSites(t, sites))
			if len(problems) == 0 {
				t.Fatalf("%s: one site changed and nothing noticed", tc.name)
			}
			if !strings.Contains(problems[0], tc.want) {
				t.Errorf("problem = %q, want it to name %q", problems[0], tc.want)
			}
		})
	}
}

func TestAFactWithNowhereToCompareIsAProblemNotASilentPass(t *testing.T) {
	t.Parallel()
	sites := nodeSites("172.17.0.1", "172.17.0.0/16", "9000", "runsc", "runsc")
	sites["infra/deploy/sandbox/bin/skillhub-bootstrap"] = "#!/bin/sh\necho nothing to read here\n"

	problems := sandboxNodeFactProblems(writeSites(t, sites))
	if len(problems) == 0 {
		t.Fatal("a site that stopped carrying the value passed; the check would go quiet exactly when it stops working")
	}
	if !strings.Contains(strings.Join(problems, "\n"), "no site to compare") {
		t.Errorf("problems = %v, want them to say the value could not be found", problems)
	}
}
