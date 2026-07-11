package com.aandi.gateway.security

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponseWriter
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.beans.factory.annotation.Value
import org.springframework.http.HttpStatus
import org.springframework.http.MediaType
import org.springframework.core.convert.converter.Converter
import org.springframework.security.authentication.AbstractAuthenticationToken
import org.springframework.security.config.annotation.web.reactive.EnableWebFluxSecurity
import org.springframework.security.config.web.server.ServerHttpSecurity
import org.springframework.security.core.authority.SimpleGrantedAuthority
import org.springframework.security.oauth2.core.DelegatingOAuth2TokenValidator
import org.springframework.security.oauth2.jose.jws.MacAlgorithm
import org.springframework.security.oauth2.jwt.Jwt
import org.springframework.security.oauth2.jwt.JwtTimestampValidator
import org.springframework.security.oauth2.jwt.NimbusReactiveJwtDecoder
import org.springframework.security.oauth2.jwt.ReactiveJwtDecoder
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken
import org.springframework.security.web.server.SecurityWebFilterChain
import org.springframework.web.cors.CorsConfiguration
import org.springframework.web.cors.reactive.CorsConfigurationSource
import org.springframework.web.cors.reactive.UrlBasedCorsConfigurationSource
import org.springframework.web.server.ServerWebExchange
import reactor.core.publisher.Mono
import java.nio.charset.StandardCharsets
import java.time.Duration
import javax.crypto.spec.SecretKeySpec

@Configuration
@EnableWebFluxSecurity
class SecurityConfig(
    private val responseWriter: GatewayResponseWriter,
    private val bearerTokenAuthenticationConverter: GatewayBearerTokenAuthenticationConverter
) {

    @Bean
    fun corsConfigurationSource(
        @Value("\${CORS_ALLOWED_ORIGIN_PATTERNS:https://*}") allowedOriginPatternsRaw: String
    ): CorsConfigurationSource {
        val allowedOriginPatterns = allowedOriginPatternsRaw
            .split(",")
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .ifEmpty { listOf("https://*") }

        val config = CorsConfiguration().apply {
            this.allowedOriginPatterns = allowedOriginPatterns
            this.allowedMethods = listOf("GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS")
            this.allowedHeaders = listOf("*")
            this.exposedHeaders = listOf("X-Auth-Context-Cache")
            this.allowCredentials = false
            this.maxAge = 3600
        }

        return UrlBasedCorsConfigurationSource().also { source ->
            source.registerCorsConfiguration("/**", config)
        }
    }

    @Bean
    @ConditionalOnProperty(name = ["gateway.auth.enabled"], havingValue = "true", matchIfMissing = true)
    fun authenticatedSecurityFilterChain(
        http: ServerHttpSecurity,
        jwtDecoder: ReactiveJwtDecoder
    ): SecurityWebFilterChain {
        return http
            .csrf { it.disable() }
            .cors { }
            .httpBasic { it.disable() }
            .formLogin { it.disable() }
            .authorizeExchange { exchanges ->
                GlobalAccessPolicyCatalog.validate()
                GlobalAccessPolicyCatalog.rules.forEach { rule ->
                    val access = when (val matcher = rule.matcher) {
                        AccessMatcherContract.AnyExchange -> exchanges.anyExchange()
                        is AccessMatcherContract.Paths -> {
                            val paths = matcher.paths.toTypedArray()
                            if (matcher.method == null) {
                                exchanges.pathMatchers(*paths)
                            } else {
                                exchanges.pathMatchers(matcher.method, *paths)
                            }
                        }
                    }

                    when (val requirement = rule.requirement) {
                        AccessRequirement.PermitAll -> access.permitAll()
                        AccessRequirement.Authenticated -> access.authenticated()
                        is AccessRequirement.AnyRole -> access.hasAnyRole(
                            *requirement.roles.map(UserRole::name).toTypedArray()
                        )
                    }
                }
            }
            .exceptionHandling { exceptions ->
                exceptions.authenticationEntryPoint { exchange, _ ->
                    responseWriter.writeError(exchange, GatewayErrorCode.AUTHENTICATION_FAILED)
                }
                exceptions.accessDeniedHandler { exchange, _ ->
                    responseWriter.writeError(exchange, GatewayErrorCode.ACCESS_DENIED)
                }
            }
            .oauth2ResourceServer { oauth2 ->
                oauth2.bearerTokenConverter(bearerTokenAuthenticationConverter)
                oauth2.jwt { jwt ->
                    jwt.jwtDecoder(jwtDecoder)
                    jwt.jwtAuthenticationConverter(jwtAuthenticationConverter())
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
    fun jwtDecoder(jwtPolicy: JwtPolicyProperties): ReactiveJwtDecoder {
        val secret = jwtPolicy.secret
        require(secret.toByteArray(StandardCharsets.UTF_8).size >= 32) {
            "security.jwt.secret must be at least 32 bytes"
        }

        val secretKey = SecretKeySpec(secret.toByteArray(StandardCharsets.UTF_8), "HmacSHA256")
        val decoder = NimbusReactiveJwtDecoder.withSecretKey(secretKey)
            .macAlgorithm(MacAlgorithm.HS256)
            .build()

        val timestampValidator = JwtTimestampValidator(Duration.ofSeconds(jwtPolicy.clockSkewSeconds))
        val issuerValidator = org.springframework.security.oauth2.jwt.JwtIssuerValidator(jwtPolicy.issuer)
        val audienceValidator = RequiredAudienceValidator(jwtPolicy.audience)
        val claimsValidator = AccessTokenClaimsValidator(Duration.ofSeconds(jwtPolicy.clockSkewSeconds))

        decoder.setJwtValidator(
            DelegatingOAuth2TokenValidator(timestampValidator, issuerValidator, audienceValidator, claimsValidator)
        )
        return decoder
    }

    private fun jwtAuthenticationConverter(): Converter<Jwt, Mono<AbstractAuthenticationToken>> {
        return Converter { jwt ->
            val role = UserRole.fromClaim(jwt.getClaimAsString("role"))
            val authorities = role?.grantedAuthorities() ?: listOf(SimpleGrantedAuthority("ROLE_USER"))
            Mono.just(JwtAuthenticationToken(jwt, authorities, jwt.subject))
        }
    }
}
