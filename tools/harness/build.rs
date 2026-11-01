include!("src/key.rs");

fn main() {
    let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
    let key = tree_key(dir, is_rust_build_input, &[]).expect("hash the harness source");
    println!("cargo:rustc-env=HARNESS_SOURCE_KEY={key:016x}");
    for input in [
        "src",
        "build.rs",
        "Cargo.toml",
        "Cargo.lock",
        "rust-toolchain.toml",
    ] {
        println!("cargo:rerun-if-changed={input}");
    }
}
