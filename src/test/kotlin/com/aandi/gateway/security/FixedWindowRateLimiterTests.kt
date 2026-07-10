package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class FixedWindowRateLimiterTests {

    @Test
    fun `enforces limit for the current minute`() {
        var minute = 10L
        val limiter = FixedWindowRateLimiter(counterSlots = 128) { minute }

        assertTrue(limiter.allow("login:user-1", 2))
        assertTrue(limiter.allow("login:user-1", 2))
        assertFalse(limiter.allow("login:user-1", 2))
    }

    @Test
    fun `drops expired keys when the minute changes`() {
        var minute = 10L
        val limiter = FixedWindowRateLimiter(counterSlots = 128) { minute }

        assertTrue(limiter.allow("login:user-1", 1))
        assertFalse(limiter.allow("login:user-1", 1))
        minute = 11L
        assertTrue(limiter.allow("login:user-1", 1))
    }

    @Test
    fun `cardinality churn does not reset an existing key counter`() {
        val limiter = FixedWindowRateLimiter(counterSlots = 128) { 10L }

        assertTrue(limiter.allow("login:user-1", 2))
        assertTrue(limiter.allow("login:user-1", 2))
        repeat(5_000) { index ->
            limiter.allow("login:churn-$index", 2)
        }

        assertFalse(limiter.allow("login:user-1", 2))
        assertEquals(128 * 4, limiter.counterSlotCount())
    }
}
