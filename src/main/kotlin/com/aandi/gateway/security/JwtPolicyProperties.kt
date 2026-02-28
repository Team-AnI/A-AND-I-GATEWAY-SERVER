package com.aandi.gateway.security

import org.springframework.boot.context.properties.ConfigurationProperties

@ConfigurationProperties(prefix = "security.jwt")
data class JwtPolicyProperties(
    val issuer: String = "http://localhost:9000",
    val audience: String = "aandi-gateway",
    val secret: String = "local-dev-jwt-secret-must-be-at-least-32-bytes",
    val clockSkewSeconds: Long = 30
)
