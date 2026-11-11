const COMMANDS: &[&str] = &["available", "speak", "stop", "poll"];

fn main() {
  tauri_plugin::Builder::new(COMMANDS)
    .android_path("android")
    .build();
}
