//! FCM push for the Treehouse Android app, called from the aterm page.
//! Contract: tooling-aterm-client/references/android-push.md, pinned by the tests.

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
use desktop::Push;
#[cfg(mobile)]
use mobile::Push;

pub use error::{Error, Result};
pub use models::{Registered, Tap};

/// Extensions to [`tauri::App`] and [`tauri::AppHandle`] for push.
pub trait PushExt<R: Runtime> {
  fn push(&self) -> &Push<R>;
}

impl<R: Runtime, T: Manager<R>> PushExt<R> for T {
  fn push(&self) -> &Push<R> {
    self.state::<Push<R>>().inner()
  }
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
  Builder::new("push")
    .invoke_handler(tauri::generate_handler![commands::register, commands::token, commands::take_tap])
    .setup(|app, api| {
      #[cfg(mobile)]
      let push = mobile::init(app, api)?;
      #[cfg(desktop)]
      let push = desktop::init(app, api)?;
      app.manage(push);
      Ok(())
    })
    .build()
}
