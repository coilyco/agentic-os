const COMMANDS: &[&str] = &["available", "begin", "write", "finish", "cancel"];

fn main() {
  tauri_plugin::Builder::new(COMMANDS)
    .android_path("android")
    .build();
}
