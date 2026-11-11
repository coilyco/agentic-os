// The file name a save may take, with no Tauri in it so a test pins it.
// Kotlin's SaveRules.kt holds the same rule, and both tests carry the same cases.

/// Longest name kept, in bytes. Android and most disks stop at 255, and the unique
/// suffix needs room.
pub const MAX_NAME_BYTES: usize = 120;

/// Most files held open at once. The viewer saves one at a time, so more is a bug.
pub const MAX_OPEN: usize = 4;

/// A name safe to write: no folders, no control characters, no leading dots, never empty.
pub fn safe_name(name: &str) -> String {
  let last = name.rsplit(['/', '\\']).next().unwrap_or("");
  let cleaned: String = last
    .chars()
    .filter(|c| !c.is_control())
    .map(|c| if matches!(c, ':' | '*' | '?' | '"' | '<' | '>' | '|') { '_' } else { c })
    .collect();
  let trimmed = cleaned.trim().trim_start_matches('.').trim();
  if trimmed.is_empty() {
    return "file".into();
  }
  clip(trimmed)
}

/// Cuts a long name at a character edge and keeps the extension.
fn clip(name: &str) -> String {
  if name.len() <= MAX_NAME_BYTES {
    return name.to_string();
  }
  let (stem, ext) = match name.rfind('.') {
    Some(dot) if name.len() - dot <= 16 && dot > 0 => (&name[..dot], &name[dot..]),
    _ => (name, ""),
  };
  let room = MAX_NAME_BYTES - ext.len();
  let mut end = room.min(stem.len());
  while !stem.is_char_boundary(end) {
    end -= 1;
  }
  format!("{}{}", &stem[..end], ext)
}

/// `name`, then `name (1).ext`, `name (2).ext`, for a file that already exists.
pub fn numbered(name: &str, attempt: u32) -> String {
  if attempt == 0 {
    return name.to_string();
  }
  match name.rfind('.') {
    Some(dot) if dot > 0 => format!("{} ({}){}", &name[..dot], attempt, &name[dot..]),
    _ => format!("{name} ({attempt})"),
  }
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn a_plain_name_is_kept() {
    assert_eq!(safe_name("print-log.gcode"), "print-log.gcode");
  }

  #[test]
  fn folders_do_not_survive() {
    assert_eq!(safe_name("../../etc/passwd"), "passwd");
    assert_eq!(safe_name("C:\\Users\\kai\\a.txt"), "a.txt");
    assert_eq!(safe_name("/"), "file");
  }

  #[test]
  fn hidden_empty_and_control_names_become_something_openable() {
    assert_eq!(safe_name(".bashrc"), "bashrc");
    assert_eq!(safe_name(""), "file");
    assert_eq!(safe_name("   "), "file");
    assert_eq!(safe_name("a\u{0}b\nc.txt"), "abc.txt");
    assert_eq!(safe_name("a:b*c?.txt"), "a_b_c_.txt");
  }

  #[test]
  fn a_long_name_keeps_its_extension_and_stays_whole_characters() {
    let long = format!("{}.mp4", "é".repeat(200));
    let name = safe_name(&long);
    assert!(name.len() <= MAX_NAME_BYTES);
    assert!(name.ends_with(".mp4"));
    assert!(name.starts_with('é'));
  }

  #[test]
  fn a_taken_name_gets_a_number_before_the_extension() {
    assert_eq!(numbered("a.txt", 0), "a.txt");
    assert_eq!(numbered("a.txt", 2), "a (2).txt");
    assert_eq!(numbered("archive.tar.gz", 1), "archive.tar (1).gz");
    assert_eq!(numbered("README", 1), "README (1)");
  }
}
