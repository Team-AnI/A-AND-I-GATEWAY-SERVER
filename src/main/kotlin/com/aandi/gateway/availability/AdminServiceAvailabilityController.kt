package com.aandi.gateway.availability

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponse
import io.swagger.v3.oas.annotations.Operation
import io.swagger.v3.oas.annotations.Parameter
import io.swagger.v3.oas.annotations.media.Schema
import io.swagger.v3.oas.annotations.responses.ApiResponse
import io.swagger.v3.oas.annotations.tags.Tag
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PathVariable
import org.springframework.web.bind.annotation.PutMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController
import reactor.core.publisher.Mono

@Schema(description = "단일 백엔드 서비스의 가용성 상태")
data class ServiceAvailabilityView(
    @field:Schema(description = "백엔드 서비스 식별자", example = "ONLINE_JUDGE", allowableValues = ["ONLINE_JUDGE", "REPORT"])
    val service: String,
    @field:Schema(description = "현재 활성화 여부 (false면 게이트웨이가 해당 서비스 라우팅을 차단)", example = "true")
    val enabled: Boolean
) {
    companion object {
        fun from(state: ServiceAvailabilityState): ServiceAvailabilityView {
            return ServiceAvailabilityView(state.service.name, state.enabled)
        }
    }
}

@Schema(description = "전체 백엔드 서비스 가용성 목록")
data class ServiceAvailabilityListResponse(
    @field:Schema(description = "백엔드 서비스 가용성 상태 목록")
    val services: List<ServiceAvailabilityView>
)

@Schema(description = "서비스 가용성 변경 요청 본문")
data class ServiceAvailabilityUpdateRequest(
    @field:Schema(
        description = "활성화 여부. true=정상 라우팅, false=게이트웨이가 503으로 차단",
        example = "false",
        requiredMode = Schema.RequiredMode.REQUIRED
    )
    val enabled: Boolean?
)

@Tag(
    name = "Admin / Service Availability",
    description = "비시즌 비용 절감을 위해 백엔드 서비스(채점/리포트)와의 통신을 게이트웨이 단에서 차단/해제하는 어드민 토글 API. " +
        "차단 시 게이트웨이는 해당 서비스로 라우팅되는 모든 요청에 즉시 503 SERVICE_TEMPORARILY_DISABLED를 반환합니다."
)
@RestController
@RequestMapping("/v2/admin/service-availability")
class AdminServiceAvailabilityController(
    private val availabilityService: BackendServiceAvailabilityService
) {

    @Operation(
        summary = "백엔드 서비스 가용성 목록 조회",
        description = "ONLINE_JUDGE, REPORT 두 서비스의 현재 토글 상태를 반환합니다. " +
            "Redis에 저장된 값이 없으면 기본값 true(활성화)로 응답합니다.",
        responses = [
            ApiResponse(responseCode = "200", description = "현재 가용성 스냅샷"),
            ApiResponse(responseCode = "401", description = "인증 실패"),
            ApiResponse(responseCode = "403", description = "ADMIN 권한 없음")
        ]
    )
    @GetMapping
    fun list(): Mono<ResponseEntity<GatewayResponse<ServiceAvailabilityListResponse>>> {
        return availabilityService.listAll().map { states ->
            ResponseEntity.ok(
                GatewayResponse.success(
                    ServiceAvailabilityListResponse(states.map(ServiceAvailabilityView::from))
                )
            )
        }
    }

    @Operation(
        summary = "백엔드 서비스 가용성 토글 변경",
        description = "특정 서비스를 활성화/비활성화합니다. 변경은 즉시 모든 게이트웨이 인스턴스에 반영됩니다.",
        responses = [
            ApiResponse(responseCode = "200", description = "변경된 가용성 상태"),
            ApiResponse(responseCode = "400", description = "요청 본문에 enabled가 없음"),
            ApiResponse(responseCode = "401", description = "인증 실패"),
            ApiResponse(responseCode = "403", description = "ADMIN 권한 없음"),
            ApiResponse(responseCode = "404", description = "알 수 없는 서비스 식별자")
        ]
    )
    @PutMapping("/{service}")
    fun update(
        @Parameter(
            description = "서비스 식별자 (대소문자 무관)",
            schema = Schema(allowableValues = ["ONLINE_JUDGE", "REPORT"])
        )
        @PathVariable("service") serviceIdentifier: String,
        @RequestBody request: ServiceAvailabilityUpdateRequest
    ): Mono<ResponseEntity<GatewayResponse<*>>> {
        val service = BackendService.fromIdentifier(serviceIdentifier)
            ?: return Mono.just(
                ResponseEntity
                    .status(HttpStatus.NOT_FOUND)
                    .body(GatewayResponse.error(GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED))
            )
        val enabled = request.enabled
            ?: return Mono.just(
                ResponseEntity
                    .status(HttpStatus.BAD_REQUEST)
                    .body(GatewayResponse.error(GatewayErrorCode.LOGIN_REQUEST_BODY_INVALID))
            )

        return availabilityService.setEnabled(service, enabled).map { state ->
            ResponseEntity.ok(GatewayResponse.success(ServiceAvailabilityView.from(state)))
        }
    }
}
