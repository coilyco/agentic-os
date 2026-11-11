//! The phone's own speech recognition, called from the aterm page's composer.
//! Shapes: tooling-aterm-client/references/android-dictation.md, pinned by the tests.

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

#[cfg(desktop)]
use desktop::Dictation;
#[cfg(mobile)]
use mobile::Dictation;

pub use error::{Error, Result};
pub use models::{Availability, Heard, Phase};

/// Extensions to [`tauri::App`] and [`tauri::AppHandle`] for dictation.
pub trait DictationExt<R: Runtime> {
  fn dictation(&self) -> &Dictation<R>;
}

impl<R: Runtime, T: Manager<R>> DictationExt<R> for T {
  fn dictation(&self) -> &Dictation<R> {
    self.state::<Dictation<R>>().inner()
  }
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
  Builder::new("dictation")
    .invoke_handler(tauri::generate_handler![
      commands::available,
      commands::start,
      commands::poll,
      commands::stop,
      commands::cancel
    ])
    .setup(|app, api| {
      #[cfg(mobile)]
      let dictation = mobile::init(app, api)?;
      #[cfg(desktop)]
      let dictation = desktop::init(app, api)?;
      app.manage(dictation);
      Ok(())
    })
    .build()
}
