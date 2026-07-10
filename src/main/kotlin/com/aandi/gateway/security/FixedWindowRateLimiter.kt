package com.aandi.gateway.security

import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.time.Instant

/**
 * Fixed-window conservative Count-Min counter.
 *
 * Hash collisions can only over-count a key, so cardinality churn cannot reset or bypass an
 * existing limit. Memory usage stays fixed at [HASH_COUNT] * [counterSlots] integer counters.
 */
internal class FixedWindowRateLimiter(
    private val counterSlots: Int,
    private val currentEpochMinute: () -> Long = { Instant.now().epochSecond / 60 }
) {
    private val counters: Array<IntArray>
    private val digest = MessageDigest.getInstance("SHA-256")
    private var activeWindowMinute: Long? = null

    init {
        require(counterSlots in 1..MAX_COUNTER_SLOTS) {
            "counterSlots must be between 1 and $MAX_COUNTER_SLOTS"
        }
        counters = Array(HASH_COUNT) { IntArray(counterSlots) }
    }

    @Synchronized
    fun allow(key: String, limit: Int): Boolean {
        if (limit <= 0) return false

        val windowMinute = currentEpochMinute()
        if (activeWindowMinute != windowMinute) {
            counters.forEach { row -> row.fill(0) }
            activeWindowMinute = windowMinute
        }

        val indexes = bucketIndexes(key)
        val currentEstimate = counters.indices.minOf { row -> counters[row][indexes[row]] }
        val nextEstimate = if (currentEstimate == Int.MAX_VALUE) Int.MAX_VALUE else currentEstimate + 1
        counters.indices.forEach { row ->
            val index = indexes[row]
            if (counters[row][index] < nextEstimate) {
                counters[row][index] = nextEstimate
            }
        }
        return nextEstimate <= limit
    }

    @Synchronized
    internal fun counterSlotCount(): Int = counters.sumOf { row -> row.size }

    private fun bucketIndexes(key: String): IntArray {
        val hash = digest.digest(key.toByteArray(StandardCharsets.UTF_8))
        return IntArray(HASH_COUNT) { row ->
            val offset = row * Int.SIZE_BYTES
            var value = 0L
            repeat(Int.SIZE_BYTES) { byteIndex ->
                value = (value shl Byte.SIZE_BITS) or (hash[offset + byteIndex].toInt() and 0xff).toLong()
            }
            (value % counterSlots).toInt()
        }
    }

    private companion object {
        private const val HASH_COUNT = 4
        private const val MAX_COUNTER_SLOTS = 1_000_000
    }
}
