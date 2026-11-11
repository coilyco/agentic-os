//! The phone's own text to speech, called from the aterm page.
//! Shapes: tooling-aterm-client/references/android-speak.md, pinned by the tests.

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
use desktop::Speak;
#[cfg(mobile)]
use mobile::Speak;

pub use error::{Error, Result};
pub use models::{Availability, Phase, Spoken};

/// Extensions to [`tauri::App`] and [`tauri::AppHandle`] for speech.
pub trait SpeakExt<R: Runtime> {
  fn speak(&self) -> &Speak<R>;
}

impl<R: Runtime, T: Manager<R>> SpeakExt<R> for T {
  fn speak(&self) -> &Speak<R> {
    self.state::<Speak<R>>().inner()
  }
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
  Builder::new("speak")
    .invoke_handler(tauri::generate_handler![
      commands::available,
      commands::speak,
      commands::stop,
      commands::poll
    ])
    .setup(|app, api| {
      #[cfg(mobile)]
      let speak = mobile::init(app, api)?;
      #[cfg(desktop)]
      let speak = desktop::init(app, api)?;
      app.manage(speak);
      Ok(())
    })
    .build()
}
