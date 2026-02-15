package com.aandi.gateway.cache

import org.springframework.beans.factory.annotation.Value
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RequestHeader
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController
import reactor.core.publisher.Mono

private const val LOGOUT_EVENT = "LOGOUT"
private const val ROLE_CHANGED_EVENT = "ROLE_CHANGED"

data class InvalidationEventRequest(
    val eventType: String,
    val subject: String
)

data class InvalidationEventResponse(
    val invalidatedKeys: Long
)

@RestController
@RequestMapping("/internal/v1/cache")
class InvalidationWebhookController(
    private val invalidationService: TokenContextInvalidationService,
    @Value("\${app.security.internal-event-token}") private val internalEventToken: String
) {

    @PostMapping("/invalidation")
    fun invalidate(
        @RequestHeader("X-Internal-Token", required = false) token: String?,
        @RequestBody request: InvalidationEventRequest
    ): Mono<ResponseEntity<InvalidationEventResponse>> {
        if (token != internalEventToken) {
            return Mono.just(ResponseEntity.status(HttpStatus.FORBIDDEN).build())
        }
        return invalidateByEvent(request)
            .map { invalidatedKeys ->
                ResponseEntity.accepted().body(InvalidationEventResponse(invalidatedKeys))
            }
    }

    private fun invalidateByEvent(request: InvalidationEventRequest): Mono<Long> {
        if (request.eventType == LOGOUT_EVENT) {
            return invalidationService.invalidateOnLogout(request.subject)
        }
        if (request.eventType == ROLE_CHANGED_EVENT) {
            return invalidationService.invalidateOnRoleChanged(request.subject)
        }
        return Mono.just(0)
    }
}
