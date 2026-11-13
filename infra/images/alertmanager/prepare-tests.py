import sys
from pathlib import Path

CREATION = "\ta, err := NewAlerts(context.Background(), time.Millisecond, 0, callback, promslog.NewNopLogger(), eventrecorder.NopRecorder(), nil, nil)\n\trequire.NoError(t, err)\n"
PENDING = "\t\treturn len(a.alerts.List()) == 0\n"
COMPLETE = "\t\treturn len(a.alerts.List()) == 0 &&\n\t\t\tcallback.alerts.Load() == 0 &&\n\t\t\tcallback.gcCount.Load() == 0\n"


def prepare(source):
    if source.count(CREATION) != 1 or source.count(PENDING) != 1:
        raise ValueError("upstream collection test changed")
    return source.replace(CREATION, CREATION + "\tt.Cleanup(a.Close)\n").replace(PENDING, COMPLETE)


def self_test():
    source = CREATION + PENDING
    expected = CREATION + "\tt.Cleanup(a.Close)\n" + COMPLETE
    assert prepare(source) == expected
    for invalid in ["", PENDING, CREATION, source + CREATION, source + PENDING, expected]:
        try:
            prepare(invalid)
        except ValueError:
            continue
        raise AssertionError("accepted changed upstream collection test")
    print("collection fixture: one preparation and six refusal checks passed")


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-test"]:
        self_test()
    elif sys.argv[1:]:
        raise SystemExit("usage: prepare-tests.py [--self-test]")
    else:
        path = Path("/tmp/alertmanager-tests/provider/mem/mem_test.go")
        path.write_text(prepare(path.read_text(encoding="utf-8")), encoding="utf-8")
