const COMMANDS: &[&str] = &["available", "start", "poll", "stop", "cancel"];

fn main() {
  tauri_plugin::Builder::new(COMMANDS)
    .android_path("android")
    .build();
}
