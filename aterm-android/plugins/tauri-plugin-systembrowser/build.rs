// The page may call `register_listener` and `remove_listener`. Android's base class
// answers them, and desktop.rs answers them on a Mac.
const COMMANDS: &[&str] = &["open", "take_return", "register_listener", "remove_listener"];

fn main() {
  tauri_plugin::Builder::new(COMMANDS)
    .android_path("android")
    .build();
}
