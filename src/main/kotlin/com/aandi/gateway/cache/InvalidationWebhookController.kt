package com.aandi.gateway.cache

import com.aandi.gateway.security.SecurityProperties
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RequestHeader
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController
import reactor.core.publisher.Mono

enum class InvalidationEventType {
    LOGOUT,
    ROLE_CHANGED
}

data class InvalidationEventRequest(
    val eventType: InvalidationEventType,
    val subject: String
)

data class InvalidationEventResponse(
    val invalidatedKeys: Long
)

@RestController
@RequestMapping("/internal/v1/cache")
class InvalidationWebhookController(
    private val invalidationService: TokenContextInvalidationService,
    private val securityProperties: SecurityProperties
) {

    @PostMapping("/invalidation")
    fun invalidate(
        @RequestHeader("X-Internal-Token", required = false) token: String?,
        @RequestBody request: InvalidationEventRequest
    ): Mono<ResponseEntity<InvalidationEventResponse>> {
        if (token != securityProperties.internalEventToken) {
            return Mono.just(ResponseEntity.status(HttpStatus.FORBIDDEN).build())
        }
        return invalidateByEvent(request)
            .map { invalidatedKeys ->
                ResponseEntity.accepted().body(InvalidationEventResponse(invalidatedKeys))
            }
    }

    private fun invalidateByEvent(request: InvalidationEventRequest): Mono<Long> {
        if (request.eventType == InvalidationEventType.LOGOUT) {
            return invalidationService.invalidateOnLogout(request.subject)
        }
        if (request.eventType == InvalidationEventType.ROLE_CHANGED) {
            return invalidationService.invalidateOnRoleChanged(request.subject)
        }
        return Mono.just(0)
    }
}
