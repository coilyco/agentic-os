package dev.coilyco.treehouse.dictation

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.media.AudioAttributes
import android.os.Bundle
import android.os.SystemClock
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import android.util.Log
import java.io.File
import java.util.Locale

// COI-3466 spike probe, debug only, never for merge. `am broadcast` drives the real
// TextToSpeech from the app process and every callback lands in logcat under ProbeTts.
class ProbeTtsReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val text = intent.getStringExtra("text") ?: "The seat is answering."
        val mode = intent.getStringExtra("mode") ?: "speak"
        Probe.run(context.applicationContext, text, mode)
    }
}

private object Probe {
    private const val TAG = "ProbeTts"
    private var tts: TextToSpeech? = null
    private var ready = false
    private var queued: (() -> Unit)? = null
    private var t0 = 0L

    private fun log(msg: String) {
        Log.i(TAG, "+${SystemClock.elapsedRealtime() - t0}ms $msg")
    }

    fun run(context: Context, text: String, mode: String) {
        t0 = SystemClock.elapsedRealtime()
        val action = { act(context, text, mode) }
        if (ready) return action()
        queued = action
        if (tts != null) return
        log("init start")
        tts = TextToSpeech(context) { status ->
            log("init status=$status (0 is success)")
            val engine = tts ?: return@TextToSpeech
            if (status != TextToSpeech.SUCCESS) return@TextToSpeech
            ready = true
            log("engine=${engine.defaultEngine} engines=${engine.engines.map { it.name }}")
            log("voices=${engine.voices?.size} us=${engine.isLanguageAvailable(Locale.US)}")
            engine.setAudioAttributes(AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_ASSISTANT)
                .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH).build())
            engine.setOnUtteranceProgressListener(object : UtteranceProgressListener() {
                override fun onStart(id: String?) = log("onStart $id")
                override fun onDone(id: String?) = log("onDone $id")
                @Deprecated("required override") override fun onError(id: String?) = log("onError $id")
                override fun onError(id: String?, code: Int) = log("onError $id code=$code")
            })
            queued?.invoke()
            queued = null
        }
    }

    private fun act(context: Context, text: String, mode: String) {
        val engine = tts ?: return
        if (mode == "file") {
            val out = File(context.cacheDir, "probe-tts.wav")
            val rc = engine.synthesizeToFile(text, Bundle(), out, "probe-file")
            log("synthesizeToFile rc=$rc file=${out.name}")
        } else {
            val rc = engine.speak(text, TextToSpeech.QUEUE_FLUSH, Bundle(), "probe-speak")
            log("speak rc=$rc")
        }
    }
}
