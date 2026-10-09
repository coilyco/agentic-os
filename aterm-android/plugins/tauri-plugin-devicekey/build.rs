const COMMANDS: &[&str] = &["status", "enroll", "sign", "forget"];

fn main() {
  tauri_plugin::Builder::new(COMMANDS)
    .android_path("android")
    .build();
}
