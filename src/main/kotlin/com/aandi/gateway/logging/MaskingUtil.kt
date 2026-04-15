package com.aandi.gateway.logging

import org.springframework.http.HttpHeaders
import org.springframework.util.MultiValueMap

object MaskingUtil {
    private val fullMaskKeys = setOf("password", "accessToken", "refreshToken")
    private val partialMaskKeys = setOf("loginId", "username")

    fun maskAuthenticate(value: String?): String? {
        val raw = value?.trim().orEmpty()
        if (raw.isBlank()) {
            return null
        }
        return if (raw.startsWith("Bearer ", ignoreCase = true)) {
            "Bearer ****"
        } else {
            "****"
        }
    }

    fun maskObject(value: Any?): Any? {
        return when (value) {
            null -> null
            is Map<*, *> -> value.entries.associate { (key, innerValue) ->
                key.toString() to maskByKey(key?.toString(), innerValue)
            }
            is Iterable<*> -> value.map { maskObject(it) }
            is Array<*> -> value.map { maskObject(it) }
            else -> value
        }
    }

    fun maskByKey(key: String?, value: Any?): Any? {
        val normalized = key?.trim().orEmpty()
        return when {
            normalized in fullMaskKeys -> FULL_MASK
            normalized in partialMaskKeys -> partialMask(value?.toString())
            else -> maskObject(value)
        }
    }

    fun partialMask(value: String?): String? {
        val raw = value?.trim().orEmpty()
        if (raw.isBlank()) {
            return raw.ifBlank { null }
        }
        val visibleLength = raw.length.coerceAtMost(3)
        return raw.take(visibleLength) + "*".repeat((raw.length - visibleLength).coerceAtLeast(0))
    }

    fun firstHeader(headers: HttpHeaders, vararg names: String): String? {
        return names.asSequence()
            .mapNotNull { headers.getFirst(it) ?: headers.entries.firstOrNull { entry -> entry.key.equals(it, ignoreCase = true) }?.value?.firstOrNull() }
            .firstOrNull { !it.isNullOrBlank() }
    }

    fun toSingleValueMap(source: MultiValueMap<String, String>): Map<String, Any?> {
        return source.entries.associate { (key, values) ->
            key to when (values.size) {
                0 -> null
                1 -> maskByKey(key, values.first())
                else -> values.map { maskByKey(key, it) }
            }
        }
    }

    private const val FULL_MASK = "****"
}
