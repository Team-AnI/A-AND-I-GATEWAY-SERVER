package com.aandi.gateway.security

import org.springframework.security.oauth2.core.OAuth2Error
import org.springframework.security.oauth2.core.OAuth2TokenValidator
import org.springframework.security.oauth2.core.OAuth2TokenValidatorResult
import org.springframework.security.oauth2.jwt.Jwt
import java.time.Clock
import java.time.Duration
import java.util.UUID

class AccessTokenClaimsValidator(
    private val clockSkew: Duration,
    private val clock: Clock = Clock.systemUTC()
) : OAuth2TokenValidator<Jwt> {

    override fun validate(token: Jwt): OAuth2TokenValidatorResult {
        val tokenType = token.getClaimAsString("token_type")
        if (tokenType != "ACCESS") {
            return failure("token_type must be ACCESS")
        }

        if (!isUuid(token.subject)) {
            return failure("sub must be UUID")
        }

        val role = UserRole.fromClaim(token.getClaimAsString("role"))
        if (role == null) {
            return failure("role must be one of USER, ORGANIZER, ADMIN")
        }

        if (token.id.isNullOrBlank()) {
            return failure("jti is required")
        }

        val issuedAt = token.issuedAt ?: return failure("iat is required")
        val nowWithSkew = clock.instant().plus(clockSkew)
        if (issuedAt.isAfter(nowWithSkew)) {
            return failure("iat cannot be in the future")
        }

        return OAuth2TokenValidatorResult.success()
    }

    private fun isUuid(subject: String?): Boolean {
        if (subject.isNullOrBlank()) return false
        return runCatching { UUID.fromString(subject) }.isSuccess
    }

    private fun failure(description: String): OAuth2TokenValidatorResult {
        return OAuth2TokenValidatorResult.failure(
            OAuth2Error("invalid_token", description, null)
        )
    }
}
