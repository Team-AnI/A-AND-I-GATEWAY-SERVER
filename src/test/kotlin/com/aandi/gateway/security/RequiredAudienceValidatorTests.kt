package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.security.oauth2.jwt.Jwt
import java.time.Instant
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class RequiredAudienceValidatorTests {

    @Test
    fun `accepts jwt when audience includes required value`() {
        val validator = RequiredAudienceValidator("aandi-gateway")
        val jwt = jwtWithAudience(listOf("aandi-gateway", "report-service"))

        val result = validator.validate(jwt)

        assertTrue(result.errors.isEmpty())
    }

    @Test
    fun `rejects jwt when required audience is missing`() {
        val validator = RequiredAudienceValidator("aandi-gateway")
        val jwt = jwtWithAudience(listOf("report-service"))

        val result = validator.validate(jwt)

        assertFalse(result.errors.isEmpty())
    }

    private fun jwtWithAudience(audience: List<String>): Jwt {
        val now = Instant.now()
        return Jwt.withTokenValue("token")
            .header("alg", "none")
            .claim("sub", "user-1")
            .claim("aud", audience)
            .issuedAt(now)
            .expiresAt(now.plusSeconds(3600))
            .build()
    }
}
