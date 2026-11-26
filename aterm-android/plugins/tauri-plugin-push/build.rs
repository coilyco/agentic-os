// `register_listener` and `remove_listener` carry the `tokenRefresh` and `tap` events.
// Android's base plugin class answers them, and a desktop build has no events.
const COMMANDS: &[&str] = &["register", "token", "take_tap", "register_listener", "remove_listener"];

fn main() {
  tauri_plugin::Builder::new(COMMANDS)
    .android_path("android")
    .build();
}
