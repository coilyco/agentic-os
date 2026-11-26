use std::marker::PhantomData;

use tauri::{AppHandle, Runtime, plugin::PluginApi};

use crate::error::{Error, Result};
use crate::models::{Registered, Tap};

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  _api: PluginApi<R, C>,
) -> Result<Push<R>> {
  Ok(Push(PhantomData))
}

/// A desktop build has no FCM, so push is off. It exists so the Mac shell builds and the
/// crate tests on a laptop. `fn() -> R` keeps it Send and Sync, which `manage` demands.
pub struct Push<R: Runtime>(PhantomData<fn() -> R>);

impl<R: Runtime> Push<R> {
  pub fn register(&self) -> Result<Registered> {
    Err(Error::push_off("this app has no push service"))
  }

  pub fn token(&self) -> Result<Option<String>> {
    Ok(None)
  }

  pub fn take_tap(&self) -> Result<Option<Tap>> {
    Ok(None)
  }
}
