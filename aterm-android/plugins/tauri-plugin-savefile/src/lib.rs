//! Saves a file the aterm page holds into Downloads, called from the aterm page.
//! Shapes: tooling-aterm-client/references/save-file.md, pinned by the tests.

use tauri::{
  Manager, Runtime,
  plugin::{Builder, TauriPlugin},
};

mod commands;
#[cfg(desktop)]
mod desktop;
mod error;
#[cfg(mobile)]
mod mobile;
mod models;
mod rules;

#[cfg(desktop)]
use desktop::SaveFile;
#[cfg(mobile)]
use mobile::SaveFile;

pub use error::{Error, Result};
pub use models::{Available, Begun, Saved};

/// Extensions to [`tauri::App`] and [`tauri::AppHandle`] for saving a file.
pub trait SaveFileExt<R: Runtime> {
  fn savefile(&self) -> &SaveFile<R>;
}

impl<R: Runtime, T: Manager<R>> SaveFileExt<R> for T {
  fn savefile(&self) -> &SaveFile<R> {
    self.state::<SaveFile<R>>().inner()
  }
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
  Builder::new("savefile")
    .invoke_handler(tauri::generate_handler![
      commands::available,
      commands::begin,
      commands::write,
      commands::finish,
      commands::cancel
    ])
    .setup(|app, api| {
      #[cfg(mobile)]
      let savefile = mobile::init(app, api)?;
      #[cfg(desktop)]
      let savefile = desktop::init(app, api)?;
      app.manage(savefile);
      Ok(())
    })
    .build()
}
