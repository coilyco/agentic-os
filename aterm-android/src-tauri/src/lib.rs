#[cfg(desktop)]
mod desktop;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
  let builder = tauri::Builder::default()
    .plugin(tauri_plugin_devicekey::init())
    .plugin(tauri_plugin_sharetarget::init());
  // The desktop config declares no window, so the window opens here with its seed.
  #[cfg(desktop)]
  let builder = builder.setup(desktop::open);
  builder.run(tauri::generate_context!()).expect("error while running the aterm app");
}
