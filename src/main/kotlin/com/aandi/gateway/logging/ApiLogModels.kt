package com.aandi.gateway.logging

data class ApiStructuredLog(
    val timestamp: String,
    val level: String,
    val logType: String,
    val message: String,
    val env: String,
    val service: ApiLogService,
    val trace: ApiLogTrace,
    val http: ApiLogHttp,
    val headers: ApiLogHeaders,
    val client: ApiLogClient,
    val actor: ApiLogActor,
    val request: ApiLogRequest,
    val response: ApiLogResponse,
    val tags: List<String>
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "@timestamp" to timestamp,
            "level" to level,
            "logType" to logType,
            "message" to message,
            "env" to env,
            "service" to service.toMap(),
            "trace" to trace.toMap(),
            "http" to http.toMap(),
            "headers" to headers.toMap(),
            "client" to client.toMap(),
            "actor" to actor.toMap(),
            "request" to request.toMap(),
            "response" to response.toMap(),
            "tags" to tags
        )
    }
}

data class ApiLogService(
    val name: String,
    val domainCode: Int,
    val version: String,
    val instanceId: String
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "name" to name,
            "domainCode" to domainCode,
            "version" to version,
            "instanceId" to instanceId
        )
    }
}

data class ApiLogTrace(
    val traceId: String,
    val requestId: String
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "traceId" to traceId,
            "requestId" to requestId
        )
    }
}

data class ApiLogHttp(
    val method: String,
    val path: String,
    val route: String,
    val statusCode: Int,
    val latencyMs: Long
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "method" to method,
            "path" to path,
            "route" to route,
            "statusCode" to statusCode,
            "latencyMs" to latencyMs
        )
    }
}

data class ApiLogHeaders(
    val deviceOS: String?,
    val authenticate: String?,
    val timestamp: String?,
    val salt: String?
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "deviceOS" to deviceOS,
            "Authenticate" to authenticate,
            "timestamp" to timestamp,
            "salt" to salt
        )
    }
}

data class ApiLogClient(
    val ip: String?,
    val userAgent: String?,
    val appVersion: String?
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "ip" to ip,
            "userAgent" to userAgent,
            "appVersion" to appVersion
        )
    }
}

data class ApiLogActor(
    val userId: Long?,
    val role: String?,
    val isAuthenticated: Boolean
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "userId" to userId,
            "role" to role,
            "isAuthenticated" to isAuthenticated
        )
    }
}

data class ApiLogRequest(
    val query: Map<String, Any?>,
    val pathVariables: Map<String, Any?>,
    val body: Any?
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "query" to query,
            "pathVariables" to pathVariables,
            "body" to (body ?: emptyMap<String, Any?>())
        )
    }
}

data class ApiLogResponse(
    val success: Boolean,
    val data: Any?,
    val error: ApiLogError?,
    val timestamp: String
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "success" to success,
            "data" to data,
            "error" to error?.toMap(),
            "timestamp" to timestamp
        )
    }
}

data class ApiLogError(
    val code: Int,
    val message: String,
    val value: String,
    val alert: String
) {
    fun toMap(): Map<String, Any?> {
        return linkedMapOf(
            "code" to code,
            "message" to message,
            "value" to value,
            "alert" to alert
        )
    }
}
