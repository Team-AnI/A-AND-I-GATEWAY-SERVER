package com.aandi.gateway.logging

import ch.qos.logback.classic.spi.ILoggingEvent
import ch.qos.logback.core.LayoutBase
import tools.jackson.databind.ObjectMapper
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneId

class StructuredJsonLogLayout : LayoutBase<ILoggingEvent>() {
    private val objectMapper = ObjectMapper()

    var env: String = "local"
    var serviceName: String = "gateway"
    var domainCode: Int = 1
    var version: String = "unknown"
    var instanceId: String = "unknown"

    override fun doLayout(event: ILoggingEvent): String {
        val payload = extractStructuredPayload(event) ?: genericPayload(event)
        return objectMapper.writeValueAsString(payload) + System.lineSeparator()
    }

    private fun extractStructuredPayload(event: ILoggingEvent): Map<String, Any?>? {
        return event.argumentArray
            ?.firstOrNull { it is ApiStructuredLog }
            ?.let { (it as ApiStructuredLog).toMap() }
    }

    private fun genericPayload(event: ILoggingEvent): Map<String, Any?> {
        return linkedMapOf(
            "@timestamp" to OffsetDateTime.ofInstant(Instant.ofEpochMilli(event.timeStamp), DEFAULT_ZONE_ID).toString(),
            "level" to event.level.levelStr,
            "logType" to "APPLICATION",
            "message" to event.formattedMessage,
            "env" to env,
            "service" to linkedMapOf(
                "name" to serviceName,
                "domainCode" to domainCode,
                "version" to version,
                "instanceId" to instanceId
            ),
            "trace" to linkedMapOf(
                "traceId" to null,
                "requestId" to null
            ),
            "http" to linkedMapOf(
                "method" to null,
                "path" to null,
                "route" to null,
                "statusCode" to null,
                "latencyMs" to null
            ),
            "headers" to linkedMapOf(
                "deviceOS" to null,
                "Authenticate" to null,
                "timestamp" to null,
                "salt" to null
            ),
            "client" to linkedMapOf(
                "ip" to null,
                "userAgent" to null,
                "appVersion" to null
            ),
            "actor" to linkedMapOf(
                "userId" to null,
                "role" to null,
                "isAuthenticated" to false
            ),
            "request" to linkedMapOf(
                "query" to emptyMap<String, Any?>(),
                "pathVariables" to emptyMap<String, Any?>(),
                "body" to emptyMap<String, Any?>()
            ),
            "response" to null,
            "tags" to listOf(serviceName, "application", "event", event.loggerName)
        )
    }

    companion object {
        private val DEFAULT_ZONE_ID: ZoneId = ZoneId.of("Asia/Seoul")
    }
}
