//! An Android Keystore signing key behind BiometricPrompt, called from the aterm page.
//! Shapes: tooling-aterm-client/references/android-devicekey.md, pinned by the tests.

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
use desktop::DeviceKey;
#[cfg(mobile)]
use mobile::DeviceKey;

pub use error::{Code, Error, Result};
pub use models::{Biometric, Enrolled, Signed, Status};

/// Extensions to [`tauri::App`] and [`tauri::AppHandle`] for the device key.
pub trait DeviceKeyExt<R: Runtime> {
  fn devicekey(&self) -> &DeviceKey<R>;
}

impl<R: Runtime, T: Manager<R>> DeviceKeyExt<R> for T {
  fn devicekey(&self) -> &DeviceKey<R> {
    self.state::<DeviceKey<R>>().inner()
  }
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
  Builder::new("devicekey")
    .invoke_handler(tauri::generate_handler![
      commands::status,
      commands::enroll,
      commands::sign,
      commands::forget
    ])
    .setup(|app, api| {
      #[cfg(mobile)]
      let devicekey = mobile::init(app, api)?;
      #[cfg(desktop)]
      let devicekey = desktop::init(app, api)?;
      app.manage(devicekey);
      Ok(())
    })
    .build()
}
