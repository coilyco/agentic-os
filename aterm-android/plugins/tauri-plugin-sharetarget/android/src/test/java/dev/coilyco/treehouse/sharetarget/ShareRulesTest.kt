package dev.coilyco.treehouse.sharetarget

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ShareRulesTest {
    @Test
    fun anImageTypeIsReadAndAnythingElseIsLeftOut() {
        assertTrue(ShareRules.isImage("image/png"))
        assertTrue(ShareRules.isImage("IMAGE/WEBP"))
        assertFalse(ShareRules.isImage("video/mp4"))
        assertFalse(ShareRules.isImage(null))
    }

    @Test
    fun theReasonNamesWhatWentWrongInTheOrderThePageSaysIt() {
        assertNull(ShareRules.skipReason(0, "image/png", 1_000))
        assertEquals(ShareRules.TOO_MANY, ShareRules.skipReason(ShareRules.MAX_ITEMS, "image/png", 1))
        assertEquals(ShareRules.NOT_IMAGE, ShareRules.skipReason(0, "video/mp4", 1))
        assertEquals(ShareRules.NOT_IMAGE, ShareRules.skipReason(0, null, null))
        assertEquals(ShareRules.TOO_BIG, ShareRules.skipReason(0, "image/png", ShareRules.MAX_BYTES + 1))
        // An unknown size is read, and the read itself stops at the cap.
        assertNull(ShareRules.skipReason(0, "image/png", null))
    }

    @Test
    fun aMissingNameIsMadeFromTheType() {
        assertEquals("Screenshot_1.png", ShareRules.displayName(" Screenshot_1.png ", "image/png", 0))
        assertEquals("shared-2.webp", ShareRules.displayName(null, "image/webp", 1))
        assertEquals("shared-1.img", ShareRules.displayName("", "image/", 0))
    }

    @Test
    fun sharedWordsAreKeptOnlyWhenThereAreSome() {
        assertNull(ShareRules.sharedText(null))
        assertNull(ShareRules.sharedText("   "))
        assertEquals("look at this", ShareRules.sharedText(" look at this\n"))
    }
}
