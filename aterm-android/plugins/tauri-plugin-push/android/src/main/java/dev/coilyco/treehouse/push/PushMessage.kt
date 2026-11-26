package dev.coilyco.treehouse.push

/** One FCM data message of treehouse.push.payload.v1, ready to show. */
data class PushMessage(
    val kind: String,
    val daemonId: String,
    val session: String?,
    val tag: String,
    val title: String,
    val body: String,
    val quiet: Boolean,
) {
    companion object {
        // A seat kind names its seat. `reachable` names only the machine.
        private val SEAT_KINDS = setOf("prompt", "ask", "done")
        const val FALLBACK_TITLE = "Treehouse"

        /** Null for a message missing what a tap needs, which is dropped unshown. */
        fun parse(data: Map<String, String>): PushMessage? {
            val daemonId = data["daemon_id"]?.takeIf { it.isNotBlank() } ?: return null
            val tag = data["tag"]?.takeIf { it.isNotBlank() } ?: return null
            val kind = data["kind"].orEmpty()
            val session = data["session"]?.takeIf { it.isNotBlank() }
            if (kind in SEAT_KINDS && session == null) return null
            return PushMessage(
                kind = kind,
                daemonId = daemonId,
                session = session,
                tag = tag,
                title = data["title"]?.takeIf { it.isNotBlank() } ?: FALLBACK_TITLE,
                body = data["body"].orEmpty(),
                quiet = kind == "done",
            )
        }
    }
}
