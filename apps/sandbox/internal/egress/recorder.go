package egress

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	closedConnection = "[DESTROY]"
	droppedByRule    = "skillhub-drop-"

	lineLimit = 1 << 20
)

func Record(in io.Reader, out, unreadable io.Writer) error {
	lines := bufio.NewScanner(in)
	lines.Buffer(nil, lineLimit)
	records := json.NewEncoder(out)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		record, ok := parse(line)
		switch {
		case ok:
			if err := records.Encode(record); err != nil {
				return err
			}
		case observesEgress(line):
			fmt.Fprintf(unreadable, "an egress line did not parse into a record: %s\n", line)
		}
	}
	return lines.Err()
}

func observesEgress(line string) bool {
	return strings.Contains(line, closedConnection) || strings.Contains(line, droppedByRule)
}

func parse(line string) (flow, bool) {
	if strings.HasPrefix(line, "{") {
		return parseKernelEntry(line)
	}
	return parseConnection(line)
}

func parseConnection(line string) (flow, bool) {
	if !strings.Contains(line, closedConnection) {
		return flow{}, false
	}
	at, ok := observedAt(line)
	if !ok {
		return flow{}, false
	}
	f := flow{
		SchemaVersion: schemaVersion, Record: flowRecord, At: at,
		Decision: acceptedDecision, Protocol: otherProtocol,
	}
	counted := map[string]int{}
	for _, token := range strings.Fields(line) {
		if named, ok := protocols[token]; ok && f.Protocol == otherProtocol {
			f.Protocol = named
			continue
		}
		key, value, isField := strings.Cut(token, "=")
		if !isField {
			continue
		}
		counted[key]++
		switch {
		case key == "src" && counted[key] == 1:
			f.Source = value
		case key == "dst" && counted[key] == 1:
			f.Destination = value
		case key == "dport" && counted[key] == 1:
			f.DestinationPort = int(number(value))
		case key == "packets" && counted[key] == 1:
			f.PacketsOut = number(value)
		case key == "bytes" && counted[key] == 1:
			f.BytesOut = number(value)
		case key == "packets" && counted[key] == 2:
			f.PacketsIn = number(value)
		case key == "bytes" && counted[key] == 2:
			f.BytesIn = number(value)
		}
	}
	if f.Source == "" || f.Destination == "" {
		return flow{}, false
	}
	return f, true
}

func parseKernelEntry(line string) (flow, bool) {
	var entry struct {
		Message  string `json:"MESSAGE"`
		Realtime string `json:"__REALTIME_TIMESTAMP"`
	}
	if json.Unmarshal([]byte(line), &entry) != nil {
		return flow{}, false
	}
	micros, err := strconv.ParseInt(entry.Realtime, 10, 64)
	if err != nil {
		return flow{}, false
	}
	return parseDrop(entry.Message, time.UnixMicro(micros).UTC())
}

func parseDrop(message string, at time.Time) (flow, bool) {
	f := flow{
		SchemaVersion: schemaVersion, Record: flowRecord, At: at,
		Decision: blockedDecision, Protocol: otherProtocol, PacketsOut: 1,
	}
	for _, token := range strings.Fields(message) {
		if rule, dropped := strings.CutPrefix(token, droppedByRule); dropped {
			f.BlockedBy = rule
			continue
		}
		key, value, isField := strings.Cut(token, "=")
		if !isField {
			continue
		}
		switch key {
		case "SRC":
			f.Source = value
		case "DST":
			f.Destination = value
		case "PROTO":
			if named, ok := protocols[strings.ToLower(value)]; ok {
				f.Protocol = named
			}
		case "DPT":
			f.DestinationPort = int(number(value))
		case "LEN":
			if f.BytesOut == 0 {
				f.BytesOut = number(value)
			}
		}
	}
	if f.BlockedBy == "" || f.Source == "" || f.Destination == "" {
		return flow{}, false
	}
	return f, true
}

const otherProtocol = "other"

var protocols = map[string]string{
	"tcp": "tcp", "udp": "udp", "icmp": "icmp", "icmpv6": "icmp", "ipv6-icmp": "icmp",
}

func observedAt(line string) (time.Time, bool) {
	stamp, _, bracketed := strings.Cut(strings.TrimPrefix(line, "["), "]")
	if !bracketed || !strings.HasPrefix(line, "[") {
		return time.Time{}, false
	}
	seconds, fraction, _ := strings.Cut(stamp, ".")
	epoch, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	nanos, err := strconv.ParseInt((fraction + "000000000")[:9], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(epoch, nanos).UTC(), true
}

func number(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
