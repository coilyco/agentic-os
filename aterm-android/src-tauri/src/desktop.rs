//! The desktop window: opens on this machine's daemon, with its director seat kept live.
//! Why and how: tooling-aterm-client/references/desktop.md.

use std::{process::Command, thread::sleep, time::Duration};

use serde::Deserialize;
use tauri::{App, WebviewUrl, WebviewWindowBuilder};

/// The seat a login opens on. One role per app, so another role is another build.
const DIRECTOR_ROLE: &str = "prod-director";
/// The daemon is a LaunchAgent that starts beside this app, so it may be a moment late.
const DAEMON_WAIT: Duration = Duration::from_secs(30);

#[derive(Deserialize)]
struct Roster {
  sessions: Vec<Seat>,
}

// The window no longer opens a seat, so only the tests read name and identity (COI-3579).
#[derive(Deserialize, Debug, PartialEq)]
#[cfg_attr(not(test), allow(dead_code))]
pub struct Seat {
  pub name: String,
  pub role: String,
  pub identity: String,
}

/// The first live seat of `role` in an `aterm agents --json` answer.
pub fn seat_of_role(agents_json: &str, role: &str) -> Option<Seat> {
  let roster: Roster = serde_json::from_str(agents_json).ok()?;
  roster.sessions.into_iter().find(|seat| seat.role == role)
}

/// aterm through a login shell, because a Login Item starts with a bare PATH and
/// the launcher `aterm bundles` writes resolves it the same way.
fn aterm(args: &str) -> Option<String> {
  let output = Command::new("/bin/zsh").args(["-lc", &format!("aterm {args}")]).output().ok()?;
  output.status.success().then(|| String::from_utf8_lossy(&output.stdout).into_owned())
}

fn live_director() -> Option<Seat> {
  seat_of_role(&aterm("agents --json")?, DIRECTOR_ROLE)
}

/// The live director, opened headless first when the daemon holds none. Waits for
/// a daemon that is not up yet. None leaves the window on the host list.
fn ensure_director() -> Option<Seat> {
  let mut waited = Duration::ZERO;
  loop {
    if aterm("agents --json").is_some() {
      break;
    }
    if waited >= DAEMON_WAIT {
      return None;
    }
    sleep(Duration::from_secs(1));
    waited += Duration::from_secs(1);
  }
  if let Some(seat) = live_director() {
    return Some(seat);
  }
  aterm(&format!("--headless --working-directory \"$HOME/projects\" {DIRECTOR_ROLE}"))?;
  live_director()
}

pub fn open(app: &mut App) -> Result<(), Box<dyn std::error::Error>> {
  // The director is kept live for the overview to list. No seat is opened on it, and the
  // page seeds nothing: the client names its own host at build time (COI-3476).
  ensure_director();
  WebviewWindowBuilder::new(app, "main", WebviewUrl::default())
    .title("Treehouse")
    .inner_size(1280.0, 860.0)
    .build()?;
  Ok(())
}

#[cfg(test)]
mod tests {
  use super::*;

  const ROSTER: &str = r#"{"format":"aterm.agents.v1","self":"x","sessions":[
    {"name":"eng-platform-stag-ox","role":"eng-platform","identity":"Stag-Ox","seat":"claude"},
    {"name":"prod-director-griffin-goose","role":"prod-director","identity":"Griffin-Goose","seat":"claude"}]}"#;

  #[test]
  fn finds_the_seat_of_a_role() {
    let seat = seat_of_role(ROSTER, "prod-director").unwrap();
    assert_eq!(seat.name, "prod-director-griffin-goose");
    assert_eq!(seat.identity, "Griffin-Goose");
  }

  #[test]
  fn no_seat_for_a_role_the_daemon_does_not_hold() {
    assert_eq!(seat_of_role(ROSTER, "sysadmin-senior"), None);
  }

  #[test]
  fn a_malformed_roster_is_no_seat() {
    assert_eq!(seat_of_role("not json", "prod-director"), None);
  }
}
