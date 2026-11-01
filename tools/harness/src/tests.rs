use super::*;
use std::fs::File;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;

static NEXT_DIR: AtomicUsize = AtomicUsize::new(0);

struct TempDir(PathBuf);

impl TempDir {
    fn new() -> Self {
        let n = NEXT_DIR.fetch_add(1, Ordering::Relaxed);
        let dir = env::temp_dir().join(format!("harness-test-{}-{n}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        TempDir(dir)
    }

    fn write(&self, relative: &str, contents: &str) -> PathBuf {
        let path = self.0.join(relative);
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(&path, contents).unwrap();
        path
    }
}

impl Drop for TempDir {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.0);
    }
}

fn go_key(dir: &Path) -> u64 {
    tree_key(dir, is_go_build_input, &[]).unwrap()
}

fn no_environment(_: &str) -> Option<String> {
    None
}

fn module(main: &str) -> TempDir {
    let dir = TempDir::new();
    dir.write("go.mod", "module example.com/probe\n\ngo 1.22\n");
    dir.write("main.go", main);
    dir
}

const EXIT_THREE: &str = "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(3) }\n";

#[test]
fn an_exit_code_outside_a_byte_is_a_failure_not_a_wrapped_value() {
    assert_eq!(exit_code(Some(0)), ExitCode::SUCCESS);
    assert_eq!(exit_code(Some(3)), ExitCode::from(3));
    assert_eq!(exit_code(Some(255)), ExitCode::from(255));
    assert_eq!(exit_code(Some(256)), ExitCode::FAILURE);
    assert_eq!(exit_code(Some(-1)), ExitCode::FAILURE);
    assert_eq!(exit_code(None), ExitCode::FAILURE);
}

#[test]
fn the_key_changes_with_the_program_and_not_with_tests_or_documents() {
    let dir = module("package main\n\nfunc main() {}\n");
    let base = go_key(&dir.0);

    dir.write("main_test.go", "package main\n");
    dir.write("README.md", "notes\n");
    dir.write("testdata/fixture.go", "package fixture\n");
    dir.write(".cache/stray.go", "package stray\n");
    assert_eq!(go_key(&dir.0), base, "non-build files changed the key");

    dir.write("main.go", "package main\n\nfunc main() { println() }\n");
    let edited = go_key(&dir.0);
    assert_ne!(edited, base, "an edited source kept the key");

    dir.write("cmd/extra.go", "package cmd\n");
    assert_ne!(go_key(&dir.0), edited, "an added source kept the key");
}

#[test]
fn the_key_tells_a_renamed_file_from_the_original() {
    let dir = module("package main\n\nfunc main() {}\n");
    dir.write("a.go", "package main\n");
    let before = go_key(&dir.0);
    fs::rename(dir.0.join("a.go"), dir.0.join("b.go")).unwrap();
    assert_ne!(go_key(&dir.0), before);
}

#[test]
fn the_root_is_the_nearest_directory_holding_devctl() {
    let repo = TempDir::new();
    repo.write("tools/devctl/go.mod", "module devctl\n");
    let nested = repo.0.join("apps/web/src");
    fs::create_dir_all(&nested).unwrap();
    assert_eq!(find_root(&nested).unwrap(), repo.0);
}

#[test]
fn outside_the_repository_the_root_is_refused_by_name() {
    let elsewhere = TempDir::new();
    let err = find_root(&elsewhere.0).unwrap_err();
    assert!(err.contains("no tools/devctl/go.mod above"), "{err}");
}

fn builds(cache: &Path) -> Vec<String> {
    let mut names: Vec<String> = fs::read_dir(cache)
        .unwrap()
        .map(|e| e.unwrap().file_name().to_string_lossy().into_owned())
        .collect();
    names.sort();
    names
}

fn aged_build(cache: &Path, name: &str, age_secs: u64) -> PathBuf {
    let path = cache.join(name);
    File::create(&path)
        .unwrap()
        .set_modified(SystemTime::now() - Duration::from_secs(age_secs))
        .unwrap();
    path
}

#[test]
fn pruning_keeps_the_newest_builds_up_to_the_limit() {
    let cache = TempDir::new();
    let keep = aged_build(&cache.0, "devctl-keep", 1000);
    for age in 1..CACHED_BUILDS as u64 {
        aged_build(&cache.0, &format!("devctl-{age}"), age);
    }
    prune(&cache.0, &keep);
    assert_eq!(
        builds(&cache.0).len(),
        CACHED_BUILDS,
        "a cache at the limit lost a build"
    );

    aged_build(&cache.0, "devctl-oldest", 500);
    aged_build(&cache.0, "unrelated", 900);
    prune(&cache.0, &keep);
    let left = builds(&cache.0);
    assert!(!left.contains(&"devctl-oldest".to_string()), "{left:?}");
    assert!(left.contains(&"devctl-keep".to_string()), "{left:?}");
    assert!(left.contains(&"unrelated".to_string()), "{left:?}");
    assert_eq!(left.len(), CACHED_BUILDS + 1);
}

#[test]
fn an_unchanged_source_reuses_its_build() {
    let source = module(EXIT_THREE);
    let cache = TempDir::new();
    let first = cached_devctl(&source.0, &cache.0, &no_environment).unwrap();
    let built = fs::metadata(&first).unwrap().modified().unwrap();
    let second = cached_devctl(&source.0, &cache.0, &no_environment).unwrap();
    assert_eq!(second, first);
    assert_eq!(
        fs::metadata(&second).unwrap().modified().unwrap(),
        built,
        "the build was redone"
    );
    assert_eq!(Command::new(&second).status().unwrap().code(), Some(3));
}

#[test]
fn an_edited_source_gets_a_new_build() {
    let source = module(EXIT_THREE);
    let cache = TempDir::new();
    let first = cached_devctl(&source.0, &cache.0, &no_environment).unwrap();
    source.write(
        "main.go",
        "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(4) }\n",
    );
    let second = cached_devctl(&source.0, &cache.0, &no_environment).unwrap();
    assert_ne!(second, first);
    assert_eq!(Command::new(&second).status().unwrap().code(), Some(4));
}

#[test]
fn a_source_that_does_not_build_leaves_nothing_in_the_cache() {
    let source = module("package main\n\nfunc main() { undefined() }\n");
    let cache = TempDir::new();
    let err = cached_devctl(&source.0, &cache.0, &no_environment).unwrap_err();
    assert!(
        err.contains("go build in") && err.contains("failed"),
        "{err}"
    );
    assert_eq!(builds(&cache.0), Vec::<String>::new());
}

#[test]
fn a_different_go_build_environment_gets_its_own_build() {
    let source = module(EXIT_THREE);
    let cache = TempDir::new();
    let plain = cached_devctl(&source.0, &cache.0, &no_environment).unwrap();
    let race = |name: &str| (name == "GOFLAGS").then(|| "-race".to_string());
    assert_ne!(cached_devctl(&source.0, &cache.0, &race).unwrap(), plain);
}

#[test]
fn the_harness_was_built_from_the_source_beside_it() {
    let source = Path::new(env!("CARGO_MANIFEST_DIR"));
    assert!(is_current(source, HARNESS_SOURCE_KEY).unwrap());
}

#[test]
fn an_edited_harness_source_is_not_current() {
    let dir = TempDir::new();
    dir.write(
        "Cargo.toml",
        "[package]
",
    );
    dir.write(
        "src/main.rs",
        "fn main() {}
",
    );
    let built_from = format!(
        "{:016x}",
        tree_key(&dir.0, is_rust_build_input, &[]).unwrap()
    );
    dir.write(
        "target/release/build-output.rs",
        "fn generated() {}
",
    );
    dir.write(
        "README.md",
        "notes
",
    );
    assert!(
        is_current(&dir.0, &built_from).unwrap(),
        "build output or documents made the harness stale"
    );
    dir.write(
        "src/main.rs",
        "fn main() { println!(); }
",
    );
    assert!(!is_current(&dir.0, &built_from).unwrap());
}
