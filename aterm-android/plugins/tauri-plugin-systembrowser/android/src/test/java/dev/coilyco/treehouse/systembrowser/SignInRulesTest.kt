package dev.coilyco.treehouse.systembrowser

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SignInRulesTest {
    @Test
    fun theAppsOwnSignInReturnCounts() {
        assertTrue(SignInRules.isReturn("dev.coilyco.treehouse:/signin?code=c&state=s"))
        assertTrue(SignInRules.isReturn("dev.coilyco.treehouse:/signin"))
    }

    @Test
    fun anythingElseAnotherAppSendsIsIgnored() {
        assertFalse(SignInRules.isReturn(null))
        assertFalse(SignInRules.isReturn("dev.coilyco.treehouse:/other?code=c"))
        assertFalse(SignInRules.isReturn("dev.coilyco.treehouse://signin?code=c"))
        assertFalse(SignInRules.isReturn("dev.coilyco.treehouse.desktop:/signin?code=c"))
        assertFalse(SignInRules.isReturn("https://accounts.coilyco.ai/signin"))
        assertFalse(SignInRules.isReturn("not an address"))
    }
}
