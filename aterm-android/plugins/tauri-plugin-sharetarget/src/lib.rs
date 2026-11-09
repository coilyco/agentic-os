//! Android's Share menu as a way into the aterm app, called from the aterm page.
//! Shapes: tooling-aterm-client/references/android-sharetarget.md, pinned by the tests.

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
use desktop::ShareTarget;
#[cfg(mobile)]
use mobile::ShareTarget;

pub use error::{Error, Result};
pub use models::{Reason, Shared, SharedImage, Skipped};

/// Extensions to [`tauri::App`] and [`tauri::AppHandle`] for the share target.
pub trait ShareTargetExt<R: Runtime> {
  fn sharetarget(&self) -> &ShareTarget<R>;
}

impl<R: Runtime, T: Manager<R>> ShareTargetExt<R> for T {
  fn sharetarget(&self) -> &ShareTarget<R> {
    self.state::<ShareTarget<R>>().inner()
  }
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
  Builder::new("sharetarget")
    .invoke_handler(tauri::generate_handler![commands::take])
    .setup(|app, api| {
      #[cfg(mobile)]
      let sharetarget = mobile::init(app, api)?;
      #[cfg(desktop)]
      let sharetarget = desktop::init(app, api)?;
      app.manage(sharetarget);
      Ok(())
    })
    .build()
}
