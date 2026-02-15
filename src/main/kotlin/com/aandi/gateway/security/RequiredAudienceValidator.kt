package com.aandi.gateway.security

import org.springframework.security.oauth2.core.OAuth2Error
import org.springframework.security.oauth2.core.OAuth2TokenValidator
import org.springframework.security.oauth2.core.OAuth2TokenValidatorResult
import org.springframework.security.oauth2.jwt.Jwt

class RequiredAudienceValidator(
    private val requiredAudience: String
) : OAuth2TokenValidator<Jwt> {

    override fun validate(token: Jwt): OAuth2TokenValidatorResult {
        if (token.audience.contains(requiredAudience)) {
            return OAuth2TokenValidatorResult.success()
        }
        return OAuth2TokenValidatorResult.failure(
            OAuth2Error(
                "invalid_token",
                "Missing required audience: $requiredAudience",
                null
            )
        )
    }
}
