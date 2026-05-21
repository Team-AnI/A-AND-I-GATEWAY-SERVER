package com.aandi.gateway.logging

import org.slf4j.LoggerFactory
import org.springframework.stereotype.Component

@Component
class ApiStructuredLogger {
    private val log = LoggerFactory.getLogger(API_LOGGER_NAME)

    fun log(payload: ApiStructuredLog) {
        when (payload.level) {
            "ERROR" -> log.error("{}", payload)
            "WARN" -> log.warn("{}", payload)
            else -> log.info("{}", payload)
        }
    }

    companion object {
        private const val API_LOGGER_NAME = "AANDI_API_LOG"
    }
}
