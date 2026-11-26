package dev.coilyco.treehouse.push

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.assertFalse
import org.junit.Test

class PushMessageTest {
    private val ask = mapOf(
        "kind" to "ask",
        "daemon_id" to "d1",
        "session" to "eng-platform-tender-beetle",
        "role" to "eng-platform",
        "identity" to "Tender Beetle",
        "title" to "Tender Beetle is asking",
        "body" to "Pick one",
        "tag" to "aterm-eng-platform-tender-beetle",
    )

    @Test
    fun anAskShowsLoudWithItsSeat() {
        val shown = PushMessage.parse(ask)!!
        assertEquals("eng-platform-tender-beetle", shown.session)
        assertEquals("aterm-eng-platform-tender-beetle", shown.tag)
        assertFalse(shown.quiet)
    }

    @Test
    fun doneIsQuiet() {
        assertTrue(PushMessage.parse(ask + ("kind" to "done"))!!.quiet)
    }

    @Test
    fun reachableNeedsNoSeat() {
        val shown = PushMessage.parse(mapOf("kind" to "reachable", "daemon_id" to "d1", "tag" to "treehouse-reachable-d1"))!!
        assertNull(shown.session)
        assertEquals(PushMessage.FALLBACK_TITLE, shown.title)
        assertFalse(shown.quiet)
    }

    @Test
    fun anUnknownKindStillShowsLoud() {
        assertFalse(PushMessage.parse(ask + ("kind" to "later"))!!.quiet)
    }

    @Test
    fun aMessageATapCannotUseIsDropped() {
        assertNull(PushMessage.parse(ask - "daemon_id"))
        assertNull(PushMessage.parse(ask - "tag"))
        assertNull(PushMessage.parse(ask - "session"))
        assertNull(PushMessage.parse(ask + ("session" to " ")))
    }
}
