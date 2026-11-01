pub fn tree_key(
    dir: &std::path::Path,
    is_input: fn(&str) -> bool,
    salt: &[(&str, String)],
) -> Result<u64, String> {
    use std::hash::Hasher;
    let mut files = Vec::new();
    collect_inputs(dir, is_input, &mut files)?;
    files.sort();
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    for (name, value) in salt {
        hasher.write(name.as_bytes());
        hasher.write_u8(0);
        hasher.write(value.as_bytes());
        hasher.write_u8(0);
    }
    for file in files {
        let contents = std::fs::read(&file).map_err(|e| format!("read {}: {e}", file.display()))?;
        let relative = file
            .strip_prefix(dir)
            .unwrap_or(&file)
            .to_string_lossy()
            .replace('\\', "/");
        hasher.write(relative.as_bytes());
        hasher.write_u8(0);
        hasher.write_usize(contents.len());
        hasher.write(&contents);
    }
    Ok(hasher.finish())
}

fn collect_inputs(
    dir: &std::path::Path,
    is_input: fn(&str) -> bool,
    files: &mut Vec<std::path::PathBuf>,
) -> Result<(), String> {
    for entry in std::fs::read_dir(dir).map_err(|e| format!("list {}: {e}", dir.display()))? {
        let path = entry
            .map_err(|e| format!("list {}: {e}", dir.display()))?
            .path();
        let name = path.file_name().and_then(|n| n.to_str()).unwrap_or("");
        if path.is_dir() {
            if !matches!(name, "testdata" | "target") && !name.starts_with('.') {
                collect_inputs(&path, is_input, files)?;
            }
        } else if is_input(name) {
            files.push(path);
        }
    }
    Ok(())
}

pub fn is_rust_build_input(name: &str) -> bool {
    name.ends_with(".rs") || matches!(name, "Cargo.toml" | "Cargo.lock" | "rust-toolchain.toml")
}
