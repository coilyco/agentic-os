package dev.coilyco.treehouse.sharetarget

// What a share reads and leaves out, with no Android in it so a JVM test pins it.
// The reason strings are the ones models.rs and the page switch on.
object ShareRules {
    /** Most images one message carries, as in the composer. */
    const val MAX_ITEMS = 4

    /** Read cap per item. The page shrinks a still over the host's limit. */
    const val MAX_BYTES = 25_000_000L

    const val NOT_IMAGE = "not_image"
    const val TOO_BIG = "too_big"
    const val UNREADABLE = "unreadable"
    const val TOO_MANY = "too_many"

    /** Any `image/` type is read. The client's list says which ones a seat can use. */
    fun isImage(mime: String?): Boolean = mime?.lowercase()?.startsWith("image/") == true

    /** Why this item is not read, or null to read it. */
    fun skipReason(index: Int, mime: String?, size: Long?): String? = when {
        index >= MAX_ITEMS -> TOO_MANY
        !isImage(mime) -> NOT_IMAGE
        size != null && size > MAX_BYTES -> TOO_BIG
        else -> null
    }

    /** A name to show. Android may give none, so one is made from the type. */
    fun displayName(name: String?, mime: String, index: Int): String {
        val trimmed = name?.trim().orEmpty()
        if (trimmed.isNotEmpty()) return trimmed.take(120)
        val extension = mime.substringAfter('/', "").substringBefore(';').ifEmpty { "img" }
        return "shared-${index + 1}.$extension"
    }

    /** Words shared with the picture, or null when there are none. */
    fun sharedText(text: CharSequence?): String? = text?.toString()?.trim()?.takeIf { it.isNotEmpty() }?.take(4000)
}
