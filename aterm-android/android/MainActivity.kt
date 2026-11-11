package dev.coilyco.treehouse

import android.view.View
import android.webkit.JavascriptInterface
import android.webkit.WebView
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat

// Replaces the activity Tauri generates (scripts/ci/aterm-android.sh, ime-patch).
class MainActivity : TauriActivity() {
  // The page's theme (COI-3554) sets the bar icons: dark ones on a light page.
  private inner class TreehouseTheme {
    @JavascriptInterface
    fun set(theme: String) {
      runOnUiThread {
        val bars = WindowCompat.getInsetsController(window, window.decorView)
        bars.isAppearanceLightStatusBars = theme == "light"
        bars.isAppearanceLightNavigationBars = theme == "light"
      }
    }
  }

  // Edge to edge on Android 15 puts the keyboard over the message box (COI-3575). Pad the
  // content by the part it covers, none if the window already resizes.
  override fun onWebViewCreate(webView: WebView) {
    super.onWebViewCreate(webView)
    webView.addJavascriptInterface(TreehouseTheme(), "TreehouseTheme")
    val content = findViewById<View>(android.R.id.content)
    ViewCompat.setOnApplyWindowInsetsListener(content) { view, insets ->
      val keyboard = insets.getInsets(WindowInsetsCompat.Type.ime()).bottom
      val where = IntArray(2)
      view.getLocationOnScreen(where)
      val covered = (where[1] + view.height - (view.rootView.height - keyboard)).coerceAtLeast(0)
      view.setPadding(view.paddingLeft, view.paddingTop, view.paddingRight, covered)
      insets
    }
    ViewCompat.requestApplyInsets(content)
  }
}
