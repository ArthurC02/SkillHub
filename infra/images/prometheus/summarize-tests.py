from collections import Counter, defaultdict, deque
import json
from pathlib import Path
import sys

DIAGNOSTIC_LIMIT = 20

def summarize(events, exit_status):
    counts = Counter()
    recent = defaultdict(lambda: deque(maxlen=30))
    failures = []
    for event in events:
        package = event.get('Package') or event.get('ImportPath', '')
        test, action = event.get('Test', ''), event.get('Action', '')
        key = (package, test)
        if action in ('pass', 'skip', 'fail', 'build-fail') and not package:
            raise ValueError('test result is missing its package identity')
        if action in ('output', 'build-output'):
            recent[key].append(event.get('Output', ''))
        if action == 'build-fail':
            counts['build_fail'] += 1
            failures.append({'package': package, 'test': test, 'output': ''.join(recent[key])})
        if action in ('pass', 'skip', 'fail'):
            counts[('test' if test else 'package') + '_' + action] += 1
            if action == 'fail':
                failures.append({'package': package, 'test': test, 'output': ''.join(recent[key])})
            recent.pop(key, None)
    result = {key: counts[key] for key in ('test_pass', 'test_skip', 'test_fail', 'package_pass', 'package_skip', 'package_fail', 'build_fail')}
    result['go_test_exit'] = exit_status
    result['successful'] = exit_status == 0 and result['test_pass'] > 0 and result['package_pass'] > 0 and not failures
    return result, failures

def self_test():
    passed = [{'Package': 'p', 'Test': 'TestA', 'Action': 'pass'}, {'Package': 'p', 'Action': 'pass'}]
    assert summarize(passed, 0)[0]['successful']
    assert not summarize(passed, 1)[0]['successful']
    assert not summarize([], 0)[0]['successful']
    skipped = [{'Package': 'p', 'Test': 'TestA', 'Action': 'skip'}, {'Package': 'p', 'Action': 'pass'}]
    assert not summarize(skipped, 0)[0]['successful']
    mixed = summarize(passed + [{'Package': 'p', 'Test': 'TestB', 'Action': 'skip'}], 0)[0]
    assert mixed['successful'] and mixed['test_skip'] == 1
    assert not summarize(passed + [{'Package': 'p', 'Action': 'fail'}], 0)[0]['successful']
    build = [{'ImportPath': 'p', 'Action': 'build-output', 'Output': 'compiler diagnostic'}, {'ImportPath': 'p', 'Action': 'build-fail'}]
    result, failures = summarize(build, 1)
    assert not result['successful'] and result['build_fail'] == 1 and failures[0]['output'] == 'compiler diagnostic'
    try:
        summarize([{'Action': 'pass'}], 0)
    except ValueError:
        pass
    else:
        raise AssertionError('missing observation identity was accepted')
    print('Test reporter: eight self-checks passed')

if __name__ == '__main__':
    if sys.argv[1:] == ['--self-test']:
        self_test()
    else:
        with Path(sys.argv[1]).open() as stream:
            result, failures = summarize((json.loads(line) for line in stream if line.strip()), int(sys.argv[2]))
        Path('/tmp/test-summary.json').write_text(json.dumps(result, indent=2))
        for failure in failures[:DIAGNOSTIC_LIMIT]:
            print(json.dumps(failure), file=sys.stderr)
        if len(failures) > DIAGNOSTIC_LIMIT:
            print('Remaining failures are recorded in test-results.jsonl: ' + str(len(failures) - DIAGNOSTIC_LIMIT), file=sys.stderr)
        print('Main-module upstream test summary: ' + json.dumps(result), flush=True)
        raise SystemExit(0 if result['successful'] else 1)
