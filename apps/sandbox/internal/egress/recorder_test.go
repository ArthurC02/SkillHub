package egress

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	acceptedLine = "[1789722012.481204]\t[DESTROY] ipv4     2 tcp      6 " +
		"src=172.17.0.4 dst=10.0.0.3 sport=52134 dport=4000 packets=18 bytes=2411 " +
		"src=10.0.0.3 dst=172.17.0.4 sport=4000 dport=52134 packets=16 bytes=18942 " +
		"[ASSURED] mark=0 delta-time=12 use=1"

	dnsDropLine = `{"__REALTIME_TIMESTAMP":"1789722013002911","_TRANSPORT":"kernel",` +
		`"MESSAGE":"skillhub-drop-dns IN=docker0 OUT=ens3 SRC=172.17.0.4 DST=1.1.1.1 ` +
		`LEN=68 TOS=0x00 PREC=0x00 TTL=63 ID=4321 PROTO=UDP SPT=41234 DPT=53 LEN=48"}`
)

func at(seconds int64, nanos int) time.Time { return time.Unix(seconds, int64(nanos)).UTC() }

func TestAnAcceptedConnectionIsRecordedWithBothDirectionsCounted(t *testing.T) {
	got, ok := parse(acceptedLine)
	if !ok {
		t.Fatal("a closed connection produced no record")
	}
	want := flow{
		SchemaVersion: "1.0", Record: "egress_flow", At: at(1789722012, 481204000),
		Decision: "accepted", Protocol: "tcp",
		Source: "172.17.0.4", Destination: "10.0.0.3", DestinationPort: 4000,
		PacketsOut: 18, BytesOut: 2411, PacketsIn: 16, BytesIn: 18942,
	}
	if got != want {
		t.Errorf("record = %+v, want %+v", got, want)
	}
}

func TestAConnectionNobodyAnsweredCountsNothingComingBack(t *testing.T) {
	line := "[1789722020.000000] [DESTROY] ipv4     2 udp      17 " +
		"src=172.17.0.4 dst=10.0.0.3 sport=5000 dport=4000 packets=3 bytes=180 " +
		"[UNREPLIED] src=10.0.0.3 dst=172.17.0.4 sport=4000 dport=5000 packets=0 bytes=0 mark=0 use=1"
	got, ok := parse(line)
	if !ok {
		t.Fatal("an unanswered connection produced no record")
	}
	if got.Protocol != "udp" || got.PacketsOut != 3 || got.BytesOut != 180 {
		t.Errorf("outbound side = %s %d packets / %d bytes, want udp 3 / 180", got.Protocol, got.PacketsOut, got.BytesOut)
	}
	if got.PacketsIn != 0 || got.BytesIn != 0 {
		t.Errorf("inbound side = %d packets / %d bytes, want 0 / 0", got.PacketsIn, got.BytesIn)
	}
}

func TestAProtocolWithoutPortsLeavesThePortOutOfTheRecord(t *testing.T) {
	line := "[1789722030.500000] [DESTROY] ipv4     2 icmp     1 " +
		"src=172.17.0.4 dst=10.0.0.3 type=8 code=0 id=7 packets=1 bytes=84 " +
		"src=10.0.0.3 dst=172.17.0.4 type=0 code=0 id=7 packets=1 bytes=84 mark=0 use=1"
	got, ok := parse(line)
	if !ok {
		t.Fatal("an icmp connection produced no record")
	}
	if got.Protocol != "icmp" || got.DestinationPort != 0 {
		t.Errorf("record = %s port %d, want icmp with no port", got.Protocol, got.DestinationPort)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "destination_port") {
		t.Errorf("a protocol without ports still carried destination_port: %s", encoded)
	}
}

func TestALineThatObservesEgressButDoesNotParseIsReported(t *testing.T) {
	for _, tc := range []struct{ name, line string }{
		{"closed connection without a timestamp", strings.SplitN(acceptedLine, "\t", 2)[1]},
		{"dropped packet without a time", strings.Replace(dnsDropLine, `"1789722013002911"`, `""`, 1)},
		{"dropped packet without a source", strings.Replace(dnsDropLine, "SRC=172.17.0.4 ", "", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var records, unreadable bytes.Buffer
			if err := Record(strings.NewReader(tc.line+"\n"), &records, &unreadable); err != nil {
				t.Fatal(err)
			}
			if records.Len() != 0 {
				t.Errorf("records = %q, want none", records.String())
			}
			if !strings.Contains(unreadable.String(), "did not parse") {
				t.Errorf("unreadable report = %q, want the line reported", unreadable.String())
			}
		})
	}
}

func TestLinesThatObserveSomethingElseAreIgnoredWithoutNoise(t *testing.T) {
	for _, tc := range []struct{ name, line string }{
		{"a connection that is still open", "[1789722040.000000] [UPDATE] ipv4     2 tcp      6 " +
			"src=172.17.0.4 dst=10.0.0.3 sport=1 dport=2 packets=1 bytes=1"},
		{"an unrelated kernel message", `{"__REALTIME_TIMESTAMP":"1789722014220000",` +
			`"MESSAGE":"docker0: port 1(veth6b1c9d2) entered blocking state"}`},
		{"an empty line", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var records, unreadable bytes.Buffer
			if err := Record(strings.NewReader(tc.line+"\n"), &records, &unreadable); err != nil {
				t.Fatal(err)
			}
			if records.Len() != 0 || unreadable.Len() != 0 {
				t.Errorf("records = %q and report = %q, want both empty", records.String(), unreadable.String())
			}
		})
	}
}

func TestADroppedPacketRecordsTheRuleAndTheSinglePacketItCost(t *testing.T) {
	got, ok := parse(dnsDropLine)
	if !ok {
		t.Fatal("a dropped packet produced no record")
	}
	want := flow{
		SchemaVersion: "1.0", Record: "egress_flow", At: at(1789722013, 2911000),
		Decision: "blocked", BlockedBy: "dns", Protocol: "udp",
		Source: "172.17.0.4", Destination: "1.1.1.1", DestinationPort: 53,
		PacketsOut: 1, BytesOut: 68, PacketsIn: 0, BytesIn: 0,
	}
	if got != want {
		t.Errorf("record = %+v, want %+v", got, want)
	}
}

func TestARunAddressRecordCarriesWhatTiesFlowsToARun(t *testing.T) {
	var logged bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logged, nil))
	log.Info(Message, AddressAssigned("9f1c8f2e-6a4b-4c1d-9e3a-0b7d5c2f1a88", 2, "172.17.0.4",
		at(1789722001, 4000000))...)

	record := map[string]any{}
	if err := json.Unmarshal(logged.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"schema_version": "1.0", "record": "run_address", "state": "assigned",
		"run_id": "9f1c8f2e-6a4b-4c1d-9e3a-0b7d5c2f1a88", "attempt": float64(2),
		"address": "172.17.0.4", "at": "2026-09-18T09:00:01.004Z",
	} {
		if record[key] != want {
			t.Errorf("record[%s] = %v, want %v", key, record[key], want)
		}
	}
}

func TestReleasingAnAddressClosesTheWindowItOpened(t *testing.T) {
	var logged bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logged, nil))
	log.Info(Message, AddressReleased("9f1c8f2e-6a4b-4c1d-9e3a-0b7d5c2f1a88", 2, "172.17.0.4",
		at(1789722277, 210000000))...)

	record := map[string]any{}
	if err := json.Unmarshal(logged.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["state"] != "released" || record["at"] != "2026-09-18T09:04:37.21Z" {
		t.Errorf("record = %v, want it released at 09:04:37.21Z", record)
	}
}

func TestWhatTheNodeWritesIsTheSampleTheContractIsCheckedAgainst(t *testing.T) {
	lines, err := os.ReadFile(filepath.Join("testdata", "node-lines.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var sample, unreadable bytes.Buffer
	if err := Record(bytes.NewReader(lines), &sample, &unreadable); err != nil {
		t.Fatal(err)
	}
	if unreadable.Len() != 0 {
		t.Errorf("the node's own lines were reported as unreadable: %s", unreadable.String())
	}

	path := filepath.Join("..", "..", "..", "..", "contracts", "events", "samples", "egress", "records.jsonl")
	if out := os.Getenv("SKILLHUB_EGRESS_SAMPLE_OUT"); out != "" {
		if err := os.WriteFile(out, sample.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sample.String() != string(committed) {
		t.Errorf("the recorded stream drifted from the contract sample.\n got: %s\nwant: %s", sample.String(), committed)
	}
}
