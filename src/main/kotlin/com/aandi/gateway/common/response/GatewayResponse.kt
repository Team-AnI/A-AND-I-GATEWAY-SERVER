package com.aandi.gateway.common.response

import org.springframework.http.HttpStatus
import java.time.OffsetDateTime
import java.time.ZoneId

private const val RESPONSE_SUCCESS = "SUCCESS"
private val KOREA_ZONE_ID: ZoneId = ZoneId.of("Asia/Seoul")

data class GatewayResponse<T>(
    val success: String = RESPONSE_SUCCESS,
    val data: T?,
    val error: GatewayErrorPayload?,
    val timestamp: OffsetDateTime = OffsetDateTime.now(KOREA_ZONE_ID)
) {
    companion object {
        fun <T> success(data: T): GatewayResponse<T> {
            return GatewayResponse(
                data = data,
                error = null
            )
        }

        fun error(errorCode: GatewayErrorCode): GatewayResponse<Nothing> {
            return GatewayResponse(
                data = null,
                error = GatewayErrorPayload(
                    code = errorCode.code,
                    message = errorCode.message,
                    value = errorCode.value,
                    alert = errorCode.alert
                )
            )
        }
    }
}

data class GatewayErrorPayload(
    val code: Int,
    val message: String,
    val value: String,
    val alert: String
)

enum class GatewayErrorCode(
    val code: Int,
    val httpStatus: HttpStatus,
    val value: String,
    val message: String,
    val alert: String
) {
    HTTPS_REQUIRED(
        code = 10001,
        httpStatus = HttpStatus.FORBIDDEN,
        value = "HTTPS_REQUIRED",
        message = "이 엔드포인트는 HTTPS 연결만 허용합니다.",
        alert = "보안 연결이 필요해요. 잠시 후 다시 시도해 주세요."
    ),
    HOST_NOT_ALLOWED(
        code = 10002,
        httpStatus = HttpStatus.FORBIDDEN,
        value = "HOST_NOT_ALLOWED",
        message = "요청 Host가 게이트웨이 허용 호스트 정책에 포함되지 않았습니다.",
        alert = "현재 주소에서는 요청을 처리할 수 없어요. 공식 도메인으로 다시 접속해 주세요."
    ),
    LOGIN_RATE_LIMIT_EXCEEDED(
        code = 10003,
        httpStatus = HttpStatus.TOO_MANY_REQUESTS,
        value = "LOGIN_RATE_LIMIT_EXCEEDED",
        message = "로그인 요청 횟수 제한을 초과했습니다.",
        alert = "로그인 시도가 너무 많아요. 잠시 후 다시 시도해 주세요."
    ),
    REFRESH_RATE_LIMIT_EXCEEDED(
        code = 10004,
        httpStatus = HttpStatus.TOO_MANY_REQUESTS,
        value = "REFRESH_RATE_LIMIT_EXCEEDED",
        message = "토큰 재발급 요청 횟수 제한을 초과했습니다.",
        alert = "요청이 너무 많아요. 잠시 후 다시 시도해 주세요."
    ),
    LOGOUT_RATE_LIMIT_EXCEEDED(
        code = 10005,
        httpStatus = HttpStatus.TOO_MANY_REQUESTS,
        value = "LOGOUT_RATE_LIMIT_EXCEEDED",
        message = "로그아웃 요청 횟수 제한을 초과했습니다.",
        alert = "요청이 너무 많아요. 잠시 후 다시 시도해 주세요."
    ),
    AUTHENTICATION_FAILED(
        code = 11001,
        httpStatus = HttpStatus.UNAUTHORIZED,
        value = "AUTHENTICATION_FAILED",
        message = "인증이 필요하거나 액세스 토큰 검증에 실패했습니다.",
        alert = "로그인 후 이용해주세요."
    ),
    REFRESH_TOKEN_INVALID(
        code = 11002,
        httpStatus = HttpStatus.UNAUTHORIZED,
        value = "REFRESH_TOKEN_INVALID",
        message = "리프레시 토큰이 유효하지 않거나 `REFRESH` 타입이 아닙니다.",
        alert = "로그인이 만료되었습니다."
    ),
    INTERNAL_TOKEN_INVALID(
        code = 11003,
        httpStatus = HttpStatus.FORBIDDEN,
        value = "INTERNAL_TOKEN_INVALID",
        message = "내부 이벤트 토큰이 없거나 설정값과 일치하지 않습니다.",
        alert = "내부 요청 인증에 실패했어요."
    ),
    ACCESS_DENIED(
        code = 12001,
        httpStatus = HttpStatus.FORBIDDEN,
        value = "ACCESS_DENIED",
        message = "인증된 사용자가 이 리소스에 접근할 권한이 없습니다.",
        alert = "이 작업을 수행할 권한이 없어요."
    ),
    LOGIN_REQUEST_BODY_INVALID(
        code = 13001,
        httpStatus = HttpStatus.BAD_REQUEST,
        value = "LOGIN_REQUEST_BODY_INVALID",
        message = "로그인 요청 본문 검증에 실패했습니다. `username` 또는 `password` 값이 없거나 비어 있습니다.",
        alert = "아이디와 비밀번호를 확인해 주세요."
    ),
    REFRESH_TOKEN_REQUIRED(
        code = 13002,
        httpStatus = HttpStatus.BAD_REQUEST,
        value = "REFRESH_TOKEN_REQUIRED",
        message = "토큰 재발급 또는 로그아웃 요청에 `refreshToken` 값이 없거나 비어 있습니다.",
        alert = "로그인이 만료되었습니다."
    ),
    JSON_CONTENT_TYPE_REQUIRED(
        code = 13003,
        httpStatus = HttpStatus.UNSUPPORTED_MEDIA_TYPE,
        value = "JSON_CONTENT_TYPE_REQUIRED",
        message = "요청 `Content-Type`은 `application/json` 또는 호환되는 `+json` 형식이어야 합니다.",
        alert = "요청 형식이 올바르지 않아요. 다시 시도해 주세요."
    ),
    ENDPOINT_NOT_ALLOWLISTED(
        code = 15001,
        httpStatus = HttpStatus.NOT_FOUND,
        value = "ENDPOINT_NOT_ALLOWLISTED",
        message = "요청 메서드와 경로가 게이트웨이 허용 목록에 없습니다.",
        alert = "요청한 기능을 찾을 수 없어요."
    )
}
