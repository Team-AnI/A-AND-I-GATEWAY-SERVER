package com.aandi.gateway.security

import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.beans.factory.annotation.Value
import org.springframework.http.HttpMethod
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
class SecurityConfig {

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
            .authorizeExchange {
                it.pathMatchers(HttpMethod.OPTIONS, "/**").permitAll()
                // Public endpoints
                it.pathMatchers(HttpMethod.POST, "/v1/auth/**").permitAll()
                it.pathMatchers(HttpMethod.POST, "/v2/auth/login", "/v2/auth/refresh", "/activate").permitAll()
                it.pathMatchers(HttpMethod.POST, "/internal/v1/cache/invalidation").permitAll()
                it.pathMatchers("/api/ping/**").permitAll()
                it.pathMatchers("/v3/api-docs/**").permitAll()
                it.pathMatchers("/v2/*/v3/api-docs", "/v2/*/v3/api-docs/**").permitAll()
                it.pathMatchers("/swagger-ui.html", "/swagger-ui/**", "/v2/docs", "/v2/docs/**", "/v2/swagger-ui/index.html", "/v2/swagger-ui/**").permitAll()
                it.pathMatchers("/actuator/health", "/actuator/health/**", "/actuator/info").permitAll()

                // Auth service role-based endpoints
                it.pathMatchers(HttpMethod.GET, "/v1/me", "/v2/auth/me").hasAnyRole("USER", "ORGANIZER", "ADMIN")
                it.pathMatchers(HttpMethod.POST, "/v1/me").hasAnyRole("USER", "ORGANIZER", "ADMIN")
                it.pathMatchers(HttpMethod.PATCH, "/v1/me").hasAnyRole("USER", "ORGANIZER", "ADMIN")
                it.pathMatchers("/v1/admin/**", "/v2/auth/admin/**").hasRole("ADMIN")
                it.pathMatchers("/v2/post/admin/courses", "/v2/post/admin/courses/**").hasRole("ADMIN")
                it.pathMatchers(HttpMethod.GET, "/v1/courses", "/v1/courses/**", "/v2/post/courses", "/v2/post/courses/**")
                    .hasAnyRole("USER", "ORGANIZER", "ADMIN")

                // Blog policy
                it.pathMatchers(HttpMethod.GET, "/v1/posts", "/v1/posts/*", "/v2/post", "/v2/post/*").permitAll()
                it.pathMatchers(HttpMethod.POST, "/v1/posts", "/v2/post").hasAnyRole("ORGANIZER", "ADMIN")
                it.pathMatchers(HttpMethod.PATCH, "/v1/posts/*", "/v2/post/*").hasAnyRole("ORGANIZER", "ADMIN")
                it.pathMatchers(HttpMethod.GET, "/v1/posts/drafts", "/v2/post/drafts").hasAnyRole("ORGANIZER", "ADMIN")
                it.pathMatchers(HttpMethod.DELETE, "/v1/posts/*", "/v2/post/*").hasRole("ADMIN")
                it.pathMatchers(HttpMethod.POST, "/v1/images", "/v2/post/images").hasAnyRole("ORGANIZER", "ADMIN")

                // Any other route requires authentication
                it.anyExchange().authenticated()
            }
            .exceptionHandling { exceptions ->
                exceptions.authenticationEntryPoint { exchange, _ ->
                    val response = exchange.response
                    response.statusCode = HttpStatus.UNAUTHORIZED
                    applyCorsHeaders(exchange)
                    response.headers.contentType = MediaType.APPLICATION_JSON
                    val body = "{\"message\":\"Unauthorized\"}".toByteArray()
                    response.writeWith(Mono.just(response.bufferFactory().wrap(body)))
                }
                exceptions.accessDeniedHandler { exchange, _ ->
                    val response = exchange.response
                    response.statusCode = HttpStatus.FORBIDDEN
                    applyCorsHeaders(exchange)
                    response.headers.contentType = MediaType.APPLICATION_JSON
                    val body = "{\"message\":\"Forbidden\"}".toByteArray()
                    response.writeWith(Mono.just(response.bufferFactory().wrap(body)))
                }
            }
            .oauth2ResourceServer { oauth2 ->
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

    private fun applyCorsHeaders(exchange: ServerWebExchange) {
        val origin = exchange.request.headers.origin?.trim().orEmpty()
        if (origin.isBlank()) {
            return
        }

        val headers = exchange.response.headers
        headers.set("Access-Control-Allow-Origin", origin)
        if (headers.getFirst("Vary") == null) {
            headers.add("Vary", "Origin")
        }
    }
}
