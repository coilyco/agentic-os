package dev.coilyco.treehouse.dictation

import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.speech.RecognitionService
import android.speech.SpeechRecognizer

// Debug builds only. Answers every session with the same growing words and never listens.
// Design: references/android-dictation.md.
class FakeRecognitionService : RecognitionService() {
    private val handler = Handler(Looper.getMainLooper())
    private val phrase = listOf("run", "the", "gate", "and", "report", "back")

    private fun words(count: Int) = Bundle().apply {
        putStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION, arrayListOf(phrase.take(count).joinToString(" ")))
    }

    override fun onStartListening(recognizerIntent: Intent, listener: Callback) {
        handler.removeCallbacksAndMessages(null)
        listener.readyForSpeech(Bundle())
        listener.beginningOfSpeech()
        for (count in 1..3) handler.postDelayed({ listener.partialResults(words(count)) }, count * 500L)
        handler.postDelayed({
            listener.endOfSpeech()
            listener.results(words(phrase.size))
        }, 2500L)
    }

    override fun onStopListening(listener: Callback) {
        handler.removeCallbacksAndMessages(null)
        listener.endOfSpeech()
        listener.results(words(phrase.size))
    }

    override fun onCancel(listener: Callback) {
        handler.removeCallbacksAndMessages(null)
    }
}
