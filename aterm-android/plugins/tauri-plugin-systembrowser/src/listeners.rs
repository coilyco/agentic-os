use std::sync::Mutex;

use serde_json::Value;
use tauri::ipc::Channel;

struct Entry {
  event: String,
  channel: Channel<Value>,
  webview: String,
}

/// The channels a page opened with `addPluginListener`, which is how Kotlin's `trigger`
/// reaches it on Android. A Mac has no Kotlin, so this answers the same two commands.
#[derive(Default)]
pub struct Listeners(Mutex<Vec<Entry>>);

impl Listeners {
  fn entries(&self) -> std::sync::MutexGuard<'_, Vec<Entry>> {
    self.0.lock().unwrap_or_else(|poisoned| poisoned.into_inner())
  }

  pub fn register(&self, event: String, channel: Channel<Value>, webview: &str) {
    self.entries().push(Entry { event, channel, webview: webview.to_owned() });
  }

  pub fn remove(&self, event: &str, channel_id: u32) {
    self.entries().retain(|entry| !(entry.event == event && entry.channel.id() == channel_id));
  }

  /// A page that navigates away no longer exists, so its channels go with it.
  pub fn remove_webview(&self, webview: &str) {
    self.entries().retain(|entry| entry.webview != webview);
  }

  /// Sends to every page listening for `event`, and drops a channel that fails to write.
  pub fn emit(&self, event: &str, payload: &Value) -> usize {
    let mut delivered = 0;
    self.entries().retain(|entry| {
      if entry.event != event {
        return true;
      }
      let sent = entry.channel.send(payload.clone()).is_ok();
      delivered += usize::from(sent);
      sent
    });
    delivered
  }
}

#[cfg(test)]
mod tests {
  use std::sync::{Arc, Mutex};

  use serde_json::json;
  use tauri::ipc::InvokeResponseBody;

  use super::*;

  fn listening() -> (Channel<Value>, Arc<Mutex<Vec<String>>>) {
    let seen = Arc::new(Mutex::new(Vec::new()));
    let sink = seen.clone();
    let channel = Channel::new(move |body: InvokeResponseBody| {
      if let InvokeResponseBody::Json(text) = body {
        sink.lock().unwrap().push(text);
      }
      Ok(())
    });
    (channel, seen)
  }

  #[test]
  fn an_event_reaches_only_the_channels_listening_for_it() {
    let listeners = Listeners::default();
    let (returned, on_return) = listening();
    let (closed, on_closed) = listening();
    listeners.register("return".into(), returned, "main");
    listeners.register("closed".into(), closed, "main");
    assert_eq!(listeners.emit("return", &json!({"url": "u"})), 1);
    assert_eq!(on_return.lock().unwrap().as_slice(), [r#"{"url":"u"}"#]);
    assert!(on_closed.lock().unwrap().is_empty());
  }

  #[test]
  fn a_removed_channel_and_a_gone_page_hear_nothing() {
    let listeners = Listeners::default();
    let (kept, on_kept) = listening();
    let (removed, on_removed) = listening();
    let (gone, on_gone) = listening();
    let removed_id = removed.id();
    listeners.register("closed".into(), kept, "main");
    listeners.register("closed".into(), removed, "main");
    listeners.register("closed".into(), gone, "other");
    listeners.remove("closed", removed_id);
    listeners.remove_webview("other");
    assert_eq!(listeners.emit("closed", &json!({})), 1);
    assert_eq!(on_kept.lock().unwrap().len(), 1);
    assert!(on_removed.lock().unwrap().is_empty() && on_gone.lock().unwrap().is_empty());
  }

  #[test]
  fn an_event_with_no_listener_is_delivered_to_nobody() {
    assert_eq!(Listeners::default().emit("return", &json!({"url": "u"})), 0);
  }
}
