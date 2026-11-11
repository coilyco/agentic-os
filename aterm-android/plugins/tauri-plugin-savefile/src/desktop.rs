use std::collections::HashMap;
use std::fs::{self, File, OpenOptions};
use std::io::{ErrorKind, Write};
use std::marker::PhantomData;
use std::path::PathBuf;
use std::sync::Mutex;

use base64::{Engine, engine::general_purpose::STANDARD};
use tauri::{AppHandle, Manager, Runtime, plugin::PluginApi};

use crate::error::{Error, Result};
use crate::models::{Available, Begun, Saved};
use crate::rules::{MAX_OPEN, numbered, safe_name};

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  app: &AppHandle<R>,
  _api: PluginApi<R, C>,
) -> Result<SaveFile<R>> {
  Ok(SaveFile::in_folder(app.path().download_dir().ok(), PhantomData))
}

struct Open {
  file: File,
  path: PathBuf,
}

/// Writes into the person's Downloads folder, one file at a time per `begin`.
/// `fn() -> R` keeps it Send and Sync.
pub struct SaveFile<R: Runtime> {
  folder: Option<PathBuf>,
  open: Mutex<(u64, HashMap<String, Open>)>,
  _runtime: PhantomData<fn() -> R>,
}

fn refuse(code: &str, message: &str) -> Error {
  Error { code: code.into(), message: message.into() }
}

impl<R: Runtime> SaveFile<R> {
  fn in_folder(folder: Option<PathBuf>, _runtime: PhantomData<fn() -> R>) -> Self {
    Self { folder, open: Mutex::new((0, HashMap::new())), _runtime }
  }

  pub fn available(&self) -> Result<Available> {
    match &self.folder {
      Some(_) => Ok(Available { place: "your Downloads folder".into() }),
      None => Err(refuse("no_folder", "This Mac has no Downloads folder to save into.")),
    }
  }

  pub fn begin(&self, name: &str, _mime: &str) -> Result<Begun> {
    let folder = self.folder.as_ref().ok_or_else(|| refuse("no_folder", "This Mac has no Downloads folder to save into."))?;
    let mut held = self.open.lock().map_err(|_| refuse("other", "The save state is broken."))?;
    if held.1.len() >= MAX_OPEN {
      return Err(refuse("busy", "Too many saves are open at once."));
    }
    let base = safe_name(name);
    for attempt in 0..1000 {
      let candidate = numbered(&base, attempt);
      let path = folder.join(&candidate);
      match OpenOptions::new().write(true).create_new(true).open(&path) {
        Ok(file) => {
          held.0 += 1;
          let id = held.0.to_string();
          held.1.insert(id.clone(), Open { file, path });
          return Ok(Begun { id, name: candidate });
        }
        Err(e) if e.kind() == ErrorKind::AlreadyExists => continue,
        Err(e) => return Err(refuse("unwritable", &e.to_string())),
      }
    }
    Err(refuse("unwritable", "Too many files with that name are already in Downloads."))
  }

  pub fn write(&self, id: &str, data: &str) -> Result<()> {
    let bytes = STANDARD.decode(data).map_err(|_| refuse("bad_data", "A piece of the file was not valid."))?;
    let mut held = self.open.lock().map_err(|_| refuse("other", "The save state is broken."))?;
    let open = held.1.get_mut(id).ok_or_else(|| refuse("unknown_save", "That save is not open."))?;
    open.file.write_all(&bytes).map_err(|e| refuse("unwritable", &e.to_string()))
  }

  pub fn finish(&self, id: &str) -> Result<Saved> {
    let mut held = self.open.lock().map_err(|_| refuse("other", "The save state is broken."))?;
    let mut open = held.1.remove(id).ok_or_else(|| refuse("unknown_save", "That save is not open."))?;
    open.file.flush().map_err(|e| refuse("unwritable", &e.to_string()))?;
    let name = open.path.file_name().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default();
    Ok(Saved { name, place: "your Downloads folder".into() })
  }

  pub fn cancel(&self, id: &str) -> Result<()> {
    let mut held = self.open.lock().map_err(|_| refuse("other", "The save state is broken."))?;
    if let Some(open) = held.1.remove(id) {
      drop(open.file);
      let _ = fs::remove_file(open.path);
    }
    Ok(())
  }
}

#[cfg(test)]
mod tests {
  use super::*;
  use tauri::test::MockRuntime;

  fn scratch(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("savefile-test-{tag}-{}", std::process::id()));
    let _ = fs::remove_dir_all(&dir);
    fs::create_dir_all(&dir).unwrap();
    dir
  }

  fn plugin(dir: PathBuf) -> SaveFile<MockRuntime> {
    SaveFile::in_folder(Some(dir), PhantomData)
  }

  #[test]
  fn pieces_arrive_as_one_file_and_a_taken_name_gets_a_number() {
    let dir = scratch("pieces");
    let save = plugin(dir.clone());
    let first = save.begin("../a.txt", "text/plain").unwrap();
    assert_eq!(first.name, "a.txt");
    save.write(&first.id, "aGVs").unwrap();
    save.write(&first.id, "bG8=").unwrap();
    assert_eq!(save.finish(&first.id).unwrap().name, "a.txt");
    assert_eq!(fs::read(dir.join("a.txt")).unwrap(), b"hello");
    let second = save.begin("a.txt", "text/plain").unwrap();
    assert_eq!(second.name, "a (1).txt");
    save.cancel(&second.id).unwrap();
    assert!(!dir.join("a (1).txt").exists());
    assert_eq!(fs::read(dir.join("a.txt")).unwrap(), b"hello");
  }

  #[test]
  fn a_bad_piece_and_an_unknown_save_are_refused_in_words() {
    let dir = scratch("refuse");
    let save = plugin(dir);
    let open = save.begin("b.bin", "application/octet-stream").unwrap();
    assert_eq!(save.write(&open.id, "!!!").unwrap_err().code, "bad_data");
    assert_eq!(save.write("nope", "AAAA").unwrap_err().code, "unknown_save");
    assert_eq!(save.finish("nope").unwrap_err().code, "unknown_save");
  }

  #[test]
  fn too_many_open_saves_is_busy_and_no_folder_says_so() {
    let dir = scratch("busy");
    let save = plugin(dir);
    for index in 0..MAX_OPEN {
      save.begin(&format!("f{index}.txt"), "text/plain").unwrap();
    }
    assert_eq!(save.begin("more.txt", "text/plain").unwrap_err().code, "busy");
    let none: SaveFile<MockRuntime> = SaveFile::in_folder(None, PhantomData);
    assert_eq!(none.available().unwrap_err().code, "no_folder");
  }
}
