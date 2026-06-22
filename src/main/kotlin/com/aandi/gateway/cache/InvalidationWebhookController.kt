package com.aandi.gateway.cache

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponse
import com.aandi.gateway.security.SecurityProperties
import io.swagger.v3.oas.annotations.Operation
import io.swagger.v3.oas.annotations.Parameter
import io.swagger.v3.oas.annotations.media.Schema
import io.swagger.v3.oas.annotations.responses.ApiResponse
import io.swagger.v3.oas.annotations.tags.Tag
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RequestHeader
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController
import reactor.core.publisher.Mono

@Schema(description = "토큰 컨텍스트 캐시 무효화를 유발하는 이벤트 종류")
enum class InvalidationEventType {
    LOGOUT,
    ROLE_CHANGED
}

@Schema(description = "토큰 컨텍스트 캐시 무효화 요청 본문")
data class InvalidationEventRequest(
    @field:Schema(description = "무효화 이벤트 종류", example = "LOGOUT")
    val eventType: InvalidationEventType,
    @field:Schema(description = "무효화 대상 사용자의 JWT subject", example = "user-1234")
    val subject: String
)

@Schema(description = "토큰 컨텍스트 캐시 무효화 응답")
data class InvalidationEventResponse(
    @field:Schema(description = "삭제된 캐시 키 개수", example = "3")
    val invalidatedKeys: Long
)

@Tag(
    name = "Internal / Token Cache Invalidation",
    description = "auth-service에서 게이트웨이로 호출하는 내부 웹훅. " +
        "사용자 로그아웃/권한 변경 시 게이트웨이의 토큰 컨텍스트 캐시를 무효화합니다. " +
        "X-Internal-Token 헤더로 인증되며 일반 사용자가 호출할 엔드포인트가 아닙니다."
)
@RestController
@RequestMapping("/internal/v1/cache")
class InvalidationWebhookController(
    private val invalidationService: TokenContextInvalidationService,
    private val securityProperties: SecurityProperties
) {

    @Operation(
        summary = "토큰 컨텍스트 캐시 무효화",
        description = "로그아웃 또는 권한 변경 이벤트에 대응해 해당 subject의 캐시 항목을 모두 제거합니다.",
        responses = [
            ApiResponse(responseCode = "202", description = "무효화 처리 완료, 삭제된 키 수 반환"),
            ApiResponse(responseCode = "403", description = "X-Internal-Token 헤더가 없거나 설정값과 불일치")
        ]
    )
    @PostMapping("/invalidation")
    fun invalidate(
        @Parameter(description = "내부 이벤트 인증 토큰", required = true)
        @RequestHeader("X-Internal-Token", required = false) token: String?,
        @RequestBody request: InvalidationEventRequest
    ): Mono<ResponseEntity<GatewayResponse<*>>> {
        if (token != securityProperties.internalEventToken) {
            return Mono.just(
                ResponseEntity
                    .status(GatewayErrorCode.INTERNAL_TOKEN_INVALID.httpStatus)
                    .body(GatewayResponse.error(GatewayErrorCode.INTERNAL_TOKEN_INVALID))
            )
        }
        return invalidateByEvent(request)
            .map { invalidatedKeys ->
                ResponseEntity
                    .status(HttpStatus.ACCEPTED)
                    .body(GatewayResponse.success(InvalidationEventResponse(invalidatedKeys)))
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
