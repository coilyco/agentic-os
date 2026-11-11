package dev.coilyco.treehouse.savefile

// The file name a save may take, with no Android in it so a JVM test pins it.
// Rust's rules.rs holds the same rule, and both tests carry the same cases.
object SaveRules {
    /** Longest name kept, in bytes. The system adds its own number to a taken name. */
    const val MAX_NAME_BYTES = 120

    /** Most files held open at once. The viewer saves one at a time, so more is a bug. */
    const val MAX_OPEN = 4

    const val FALLBACK_NAME = "file"
    const val FALLBACK_MIME = "application/octet-stream"

    private const val RESERVED = ":*?\"<>|"

    /** A name safe to write: no folders, no control characters, no leading dots. */
    fun safeName(name: String?): String {
        val last = (name ?: "").split('/', '\\').last()
        val cleaned = buildString {
            for (c in last) {
                if (Character.isISOControl(c)) continue
                append(if (RESERVED.indexOf(c) >= 0) '_' else c)
            }
        }
        val trimmed = cleaned.trim().trimStart('.').trim()
        return if (trimmed.isEmpty()) FALLBACK_NAME else clip(trimmed)
    }

    /** Cuts a long name at a character edge and keeps a short extension. */
    private fun clip(name: String): String {
        if (bytes(name) <= MAX_NAME_BYTES) return name
        val dot = name.lastIndexOf('.')
        val keep = dot > 0 && bytes(name.substring(dot)) <= 16
        val stem = if (keep) name.substring(0, dot) else name
        val ext = if (keep) name.substring(dot) else ""
        val room = MAX_NAME_BYTES - bytes(ext)
        val out = StringBuilder()
        var used = 0
        var at = 0
        while (at < stem.length) {
            val point = stem.codePointAt(at)
            val width = bytes(String(Character.toChars(point)))
            if (used + width > room) break
            out.appendCodePoint(point)
            used += width
            at += Character.charCount(point)
        }
        return out.toString() + ext
    }

    private fun bytes(text: String): Int = text.toByteArray(Charsets.UTF_8).size

    /** The name's own type, or plain bytes. MediaStore adds an extension otherwise. */
    fun storedMime(typeOfExtension: String?): String = typeOfExtension?.takeIf { it.contains('/') } ?: FALLBACK_MIME

    /** The part after the last dot, lower case, or null when there is none. */
    fun extensionOf(name: String): String? =
        name.substringAfterLast('.', "").lowercase().takeIf { it.isNotEmpty() && name.lastIndexOf('.') > 0 }
}
