package com.aandi.gateway.logging

import com.aandi.gateway.common.response.GatewayErrorCode
import org.springframework.http.HttpHeaders
import org.springframework.http.HttpStatus
import org.springframework.security.core.Authentication
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken
import org.springframework.stereotype.Component
import org.springframework.web.reactive.HandlerMapping
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.util.pattern.PathPattern
import tools.jackson.databind.JsonNode
import tools.jackson.databind.ObjectMapper
import java.time.Duration
import java.time.OffsetDateTime
import java.time.ZoneId

@Component
class ApiLogFactory(
    private val objectMapper: ObjectMapper,
    private val properties: ApiLoggingProperties
) {
    fun create(exchange: ServerWebExchange, context: ApiLogContext, authentication: Authentication?): ApiStructuredLog {
        val statusCode = exchange.response.statusCode?.value() ?: HttpStatus.OK.value()
        val response = buildResponse(context.responseBody, statusCode, context)
        val success = statusCode < 400 && response.error == null
        val level = when {
            success -> INFO
            statusCode >= 500 -> ERROR
            else -> WARN
        }
        val message = when {
            success -> SUCCESS_MESSAGE
            !context.failureMessage.isNullOrBlank() -> context.failureMessage!!
            response.error != null -> response.error.message
            else -> FAILURE_MESSAGE
        }

        return ApiStructuredLog(
            timestamp = now(),
            level = level,
            logType = if (success) API_LOG_TYPE else API_ERROR_LOG_TYPE,
            message = if (success) SUCCESS_MESSAGE else message,
            env = properties.env,
            service = ApiLogService(
                name = properties.service.name,
                domainCode = properties.service.domainCode,
                version = properties.service.version,
                instanceId = properties.service.instanceId
            ),
            trace = ApiLogTrace(
                traceId = context.traceId,
                requestId = context.requestId
            ),
            http = ApiLogHttp(
                method = exchange.request.method?.name() ?: "UNKNOWN",
                path = exchange.request.path.value(),
                route = resolveRoute(exchange),
                statusCode = statusCode,
                latencyMs = Duration.between(context.startedAt, java.time.Instant.now()).toMillis()
            ),
            headers = ApiLogHeaders(
                deviceOS = MaskingUtil.firstHeader(exchange.request.headers, "deviceOS", "DeviceOS", "Device-OS", "X-Device-OS"),
                authenticate = MaskingUtil.maskAuthenticate(
                    MaskingUtil.firstHeader(exchange.request.headers, "Authenticate", HttpHeaders.AUTHORIZATION)
                ),
                timestamp = MaskingUtil.firstHeader(exchange.request.headers, "timestamp", "Timestamp"),
                salt = MaskingUtil.firstHeader(exchange.request.headers, "salt", "Salt")
            ),
            client = ApiLogClient(
                ip = resolveClientIp(exchange),
                userAgent = MaskingUtil.firstHeader(exchange.request.headers, HttpHeaders.USER_AGENT),
                appVersion = MaskingUtil.firstHeader(exchange.request.headers, "appVersion", "App-Version", "X-App-Version")
            ),
            actor = buildActor(authentication),
            request = ApiLogRequest(
                query = MaskingUtil.toSingleValueMap(exchange.request.queryParams),
                pathVariables = maskPathVariables(exchange),
                body = buildRequestBody(context.requestBody)
            ),
            response = response,
            tags = buildTags(exchange.request.path.value(), success)
        )
    }

    fun createExceptionLog(
        exchange: ServerWebExchange,
        context: ApiLogContext,
        authentication: Authentication?,
        errorCode: GatewayErrorCode,
        throwable: Throwable
    ): ApiStructuredLog {
        context.markFailure("${throwable.javaClass.simpleName}: ${throwable.message ?: errorCode.message}")
        val response = ApiLogResponse(
            success = false,
            data = null,
            error = ApiLogError(
                code = errorCode.code,
                message = errorCode.message,
                value = errorCode.value,
                alert = errorCode.alert
            ),
            timestamp = now()
        )

        return ApiStructuredLog(
            timestamp = now(),
            level = ERROR,
            logType = API_ERROR_LOG_TYPE,
            message = context.failureMessage ?: FAILURE_MESSAGE,
            env = properties.env,
            service = ApiLogService(
                name = properties.service.name,
                domainCode = properties.service.domainCode,
                version = properties.service.version,
                instanceId = properties.service.instanceId
            ),
            trace = ApiLogTrace(context.traceId, context.requestId),
            http = ApiLogHttp(
                method = exchange.request.method?.name() ?: "UNKNOWN",
                path = exchange.request.path.value(),
                route = resolveRoute(exchange),
                statusCode = errorCode.httpStatus.value(),
                latencyMs = Duration.between(context.startedAt, java.time.Instant.now()).toMillis()
            ),
            headers = ApiLogHeaders(
                deviceOS = MaskingUtil.firstHeader(exchange.request.headers, "deviceOS", "DeviceOS", "Device-OS", "X-Device-OS"),
                authenticate = MaskingUtil.maskAuthenticate(
                    MaskingUtil.firstHeader(exchange.request.headers, "Authenticate", HttpHeaders.AUTHORIZATION)
                ),
                timestamp = MaskingUtil.firstHeader(exchange.request.headers, "timestamp", "Timestamp"),
                salt = MaskingUtil.firstHeader(exchange.request.headers, "salt", "Salt")
            ),
            client = ApiLogClient(
                ip = resolveClientIp(exchange),
                userAgent = MaskingUtil.firstHeader(exchange.request.headers, HttpHeaders.USER_AGENT),
                appVersion = MaskingUtil.firstHeader(exchange.request.headers, "appVersion", "App-Version", "X-App-Version")
            ),
            actor = buildActor(authentication),
            request = ApiLogRequest(
                query = MaskingUtil.toSingleValueMap(exchange.request.queryParams),
                pathVariables = maskPathVariables(exchange),
                body = buildRequestBody(context.requestBody)
            ),
            response = response,
            tags = buildTags(exchange.request.path.value(), false)
        )
    }

    private fun buildResponse(rawBody: String, statusCode: Int, context: ApiLogContext): ApiLogResponse {
        if (rawBody.isBlank()) {
            return ApiLogResponse(
                success = statusCode < 400,
                data = if (statusCode < 400) emptyMap<String, Any?>() else null,
                error = if (statusCode < 400) null else ApiLogError(
                    code = GatewayErrorCode.INTERNAL_SERVER_ERROR.code,
                    message = context.failureMessage ?: GatewayErrorCode.INTERNAL_SERVER_ERROR.message,
                    value = GatewayErrorCode.INTERNAL_SERVER_ERROR.value,
                    alert = GatewayErrorCode.INTERNAL_SERVER_ERROR.alert
                ),
                timestamp = now()
            )
        }

        return runCatching {
            val root = objectMapper.readTree(rawBody)
            val errorNode = root.get("error")
            val error = if (errorNode != null && !errorNode.isNull) {
                ApiLogError(
                    code = errorNode.path("code").asInt(GatewayErrorCode.INTERNAL_SERVER_ERROR.code),
                    message = errorNode.path("message").asText(GatewayErrorCode.INTERNAL_SERVER_ERROR.message),
                    value = errorNode.path("value").asText(GatewayErrorCode.INTERNAL_SERVER_ERROR.value),
                    alert = errorNode.path("alert").asText(GatewayErrorCode.INTERNAL_SERVER_ERROR.alert)
                )
            } else {
                null
            }
            ApiLogResponse(
                success = error == null && statusCode < 400,
                data = root.get("data")?.takeUnless { it.isNull }?.let { MaskingUtil.maskObject(jsonNodeToValue(it)) }
                    ?: if (statusCode < 400) emptyMap<String, Any?>() else null,
                error = error,
                timestamp = root.path("timestamp").asText(now())
            )
        }.getOrElse {
            ApiLogResponse(
                success = statusCode < 400,
                data = if (statusCode < 400) mapOf("raw" to rawBody) else null,
                error = if (statusCode < 400) null else ApiLogError(
                    code = GatewayErrorCode.INTERNAL_SERVER_ERROR.code,
                    message = context.failureMessage ?: GatewayErrorCode.INTERNAL_SERVER_ERROR.message,
                    value = GatewayErrorCode.INTERNAL_SERVER_ERROR.value,
                    alert = GatewayErrorCode.INTERNAL_SERVER_ERROR.alert
                ),
                timestamp = now()
            )
        }
    }

    private fun buildRequestBody(rawBody: String): Any? {
        if (rawBody.isBlank()) {
            return emptyMap<String, Any?>()
        }

        return runCatching {
            val root = objectMapper.readTree(rawBody)
            MaskingUtil.maskObject(jsonNodeToValue(root))
        }.getOrElse {
            mapOf("raw" to rawBody)
        }
    }

    private fun buildActor(authentication: Authentication?): ApiLogActor {
        if (authentication == null || !authentication.isAuthenticated) {
            return ApiLogActor(
                userId = null,
                role = null,
                isAuthenticated = false
            )
        }

        val role = authentication.authorities
            .firstOrNull()
            ?.authority
            ?.removePrefix("ROLE_")

        return ApiLogActor(
            userId = resolveUserId(authentication),
            role = role,
            isAuthenticated = true
        )
    }

    private fun resolveUserId(authentication: Authentication): Long? {
        if (authentication is JwtAuthenticationToken) {
            authentication.token.getClaimAsString("userId")
                ?.toLongOrNull()
                ?.let { return it }
            authentication.token.subject?.toLongOrNull()?.let { return it }
        }
        return authentication.name?.toLongOrNull()
    }

    private fun maskPathVariables(exchange: ServerWebExchange): Map<String, Any?> {
        val pathVariables = exchange.getAttribute<Map<String, String>>(HandlerMapping.URI_TEMPLATE_VARIABLES_ATTRIBUTE).orEmpty()
        return pathVariables.mapValues { (key, value) -> MaskingUtil.maskByKey(key, value) }
    }

    private fun resolveClientIp(exchange: ServerWebExchange): String? {
        val forwarded = MaskingUtil.firstHeader(exchange.request.headers, "X-Forwarded-For")
        if (!forwarded.isNullOrBlank()) {
            return forwarded.substringBefore(',').trim()
        }
        return exchange.request.remoteAddress?.address?.hostAddress
    }

    private fun resolveRoute(exchange: ServerWebExchange): String {
        val matchedPattern = exchange.getAttribute<Any>(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE)
        return when (matchedPattern) {
            is PathPattern -> matchedPattern.patternString
            is String -> matchedPattern
            else -> exchange.request.path.value()
        }
    }

    private fun buildTags(path: String, success: Boolean): List<String> {
        val segments = path.split('/')
            .filter { it.isNotBlank() }
            .filterNot { it.matches(Regex("v\\d+")) }

        val primary = segments.getOrNull(0) ?: "root"
        val detail = segments.firstOrNull { it != primary && !it.startsWith("{") && !it.all(Char::isDigit) }
            ?: primary

        return listOf(
            properties.service.name,
            primary,
            if (success) "success" else "fail",
            detail
        )
    }

    private fun jsonNodeToValue(node: JsonNode): Any? {
        return when {
            node.isObject -> node.properties().associate { entry -> entry.key to jsonNodeToValue(entry.value) }
            node.isArray -> node.map { jsonNodeToValue(it) }
            node.isTextual -> node.asText()
            node.isNumber -> node.numberValue()
            node.isBoolean -> node.asBoolean()
            node.isNull -> null
            else -> node.asText()
        }
    }

    private fun now(): String = OffsetDateTime.now(KOREA_ZONE_ID).toString()

    companion object {
        private val KOREA_ZONE_ID: ZoneId = ZoneId.of("Asia/Seoul")
        private const val INFO = "INFO"
        private const val WARN = "WARN"
        private const val ERROR = "ERROR"
        private const val API_LOG_TYPE = "API"
        private const val API_ERROR_LOG_TYPE = "API_ERROR"
        private const val SUCCESS_MESSAGE = "HTTP request completed"
        private const val FAILURE_MESSAGE = "HTTP request failed"
    }
}
