package dev.coilyco.treehouse.savefile

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SaveRulesTest {
    @Test
    fun aPlainNameIsKept() {
        assertEquals("print-log.gcode", SaveRules.safeName("print-log.gcode"))
    }

    @Test
    fun foldersDoNotSurvive() {
        assertEquals("passwd", SaveRules.safeName("../../etc/passwd"))
        assertEquals("a.txt", SaveRules.safeName("C:\\Users\\kai\\a.txt"))
        assertEquals("file", SaveRules.safeName("/"))
    }

    @Test
    fun hiddenEmptyAndControlNamesBecomeSomethingOpenable() {
        assertEquals("bashrc", SaveRules.safeName(".bashrc"))
        assertEquals("file", SaveRules.safeName(""))
        assertEquals("file", SaveRules.safeName(null))
        assertEquals("file", SaveRules.safeName("   "))
        assertEquals("abc.txt", SaveRules.safeName("a\u0000b\nc.txt"))
        assertEquals("a_b_c_.txt", SaveRules.safeName("a:b*c?.txt"))
    }

    @Test
    fun aLongNameKeepsItsExtensionAndStaysWholeCharacters() {
        val name = SaveRules.safeName("é".repeat(200) + ".mp4")
        assertTrue(name.toByteArray(Charsets.UTF_8).size <= SaveRules.MAX_NAME_BYTES)
        assertTrue(name.endsWith(".mp4"))
        assertTrue(name.startsWith("é"))
    }

    @Test
    fun aMissingOrOddTypeBecomesPlainBytes() {
        assertEquals("video/mp4", SaveRules.mimeOrDefault(" video/mp4 "))
        assertEquals(SaveRules.FALLBACK_MIME, SaveRules.mimeOrDefault(null))
        assertEquals(SaveRules.FALLBACK_MIME, SaveRules.mimeOrDefault("junk"))
    }
}
