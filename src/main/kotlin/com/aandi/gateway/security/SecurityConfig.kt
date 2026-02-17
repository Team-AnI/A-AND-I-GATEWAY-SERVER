package com.aandi.gateway.security

import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty
import org.springframework.beans.factory.annotation.Value
import org.springframework.http.HttpMethod
import org.springframework.security.config.annotation.web.reactive.EnableWebFluxSecurity
import org.springframework.security.config.web.server.ServerHttpSecurity
import org.springframework.security.oauth2.core.DelegatingOAuth2TokenValidator
import org.springframework.security.oauth2.jwt.JwtValidators
import org.springframework.security.oauth2.jwt.NimbusReactiveJwtDecoder
import org.springframework.security.oauth2.jwt.ReactiveJwtDecoder
import org.springframework.security.web.server.SecurityWebFilterChain

@Configuration
@EnableWebFluxSecurity
class SecurityConfig {

    @Bean
    @ConditionalOnProperty(name = ["gateway.auth.enabled"], havingValue = "true", matchIfMissing = true)
    fun authenticatedSecurityFilterChain(
        http: ServerHttpSecurity,
        jwtDecoder: ReactiveJwtDecoder
    ): SecurityWebFilterChain {
        return http
            .csrf { it.disable() }
            .httpBasic { it.disable() }
            .formLogin { it.disable() }
            .authorizeExchange {
                it.pathMatchers(HttpMethod.POST, "/internal/v1/cache/invalidation").permitAll()
                it.pathMatchers("/actuator/health", "/actuator/health/**", "/actuator/info").permitAll()
                it.anyExchange().authenticated()
            }
            .oauth2ResourceServer { oauth2 ->
                oauth2.jwt { jwt ->
                    jwt.jwtDecoder(jwtDecoder)
                }
            }
            .build()
    }

    @Bean
    @ConditionalOnProperty(name = ["gateway.auth.enabled"], havingValue = "false")
    fun permitAllSecurityFilterChain(http: ServerHttpSecurity): SecurityWebFilterChain {
        return http
            .csrf { it.disable() }
            .httpBasic { it.disable() }
            .formLogin { it.disable() }
            .authorizeExchange {
                it.anyExchange().permitAll()
            }
            .build()
    }

    @Bean
    @ConditionalOnProperty(name = ["gateway.auth.enabled"], havingValue = "true", matchIfMissing = true)
    fun jwtDecoder(
        properties: SecurityProperties,
        @Value("\${spring.security.oauth2.resourceserver.jwt.issuer-uri}") issuerUri: String,
        @Value("\${spring.security.oauth2.resourceserver.jwt.jwk-set-uri}") jwkSetUri: String
    ): ReactiveJwtDecoder {
        val decoder = NimbusReactiveJwtDecoder.withJwkSetUri(jwkSetUri).build()
        val issuerValidator = JwtValidators.createDefaultWithIssuer(issuerUri)
        val audienceValidator = RequiredAudienceValidator(properties.requiredAudience)
        decoder.setJwtValidator(DelegatingOAuth2TokenValidator(issuerValidator, audienceValidator))
        return decoder
    }
}
