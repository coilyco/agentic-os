use std::marker::PhantomData;

use tauri::{AppHandle, Runtime, plugin::PluginApi};

use crate::error::Result;
use crate::models::Shared;

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  _api: PluginApi<R, C>,
) -> Result<ShareTarget<R>> {
  Ok(ShareTarget(PhantomData))
}

/// No Share menu on desktop, so nothing waits. `fn() -> R` keeps it Send and Sync.
pub struct ShareTarget<R: Runtime>(PhantomData<fn() -> R>);

impl<R: Runtime> ShareTarget<R> {
  pub fn take(&self) -> Result<Shared> {
    Ok(Shared::default())
  }
}
