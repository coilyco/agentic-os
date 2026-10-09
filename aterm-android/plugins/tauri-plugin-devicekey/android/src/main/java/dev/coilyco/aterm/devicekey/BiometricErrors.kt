package dev.coilyco.aterm.devicekey

import android.hardware.biometrics.BiometricPrompt

object BiometricErrors {
    /** Maps a BiometricPrompt error to one of [Codes]. Anything unlisted is `other`. */
    fun codeFor(errorCode: Int): String = when (errorCode) {
        BiometricPrompt.BIOMETRIC_ERROR_USER_CANCELED,
        BiometricPrompt.BIOMETRIC_ERROR_CANCELED -> Codes.CANCELLED

        BiometricPrompt.BIOMETRIC_ERROR_LOCKOUT,
        BiometricPrompt.BIOMETRIC_ERROR_LOCKOUT_PERMANENT -> Codes.LOCKOUT

        BiometricPrompt.BIOMETRIC_ERROR_NO_BIOMETRICS,
        BiometricPrompt.BIOMETRIC_ERROR_NO_DEVICE_CREDENTIAL -> Codes.NONE_ENROLLED

        BiometricPrompt.BIOMETRIC_ERROR_HW_NOT_PRESENT,
        BiometricPrompt.BIOMETRIC_ERROR_HW_UNAVAILABLE,
        BiometricPrompt.BIOMETRIC_ERROR_SECURITY_UPDATE_REQUIRED -> Codes.UNSUPPORTED

        else -> Codes.OTHER
    }
}
