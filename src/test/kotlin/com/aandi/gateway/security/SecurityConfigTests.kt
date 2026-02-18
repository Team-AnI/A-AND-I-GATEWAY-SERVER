package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.context.ApplicationContext
import org.springframework.http.MediaType
import org.springframework.security.core.authority.SimpleGrantedAuthority
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.mockJwt
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.springSecurity
import org.springframework.test.web.reactive.server.WebTestClient
import kotlin.test.assertNotEquals

@SpringBootTest(
    properties = [
        "POST_SERVICE_URI=http://localhost:8084",
        "AUTH_SERVICE_URI=http://localhost:9000",
        "app.security.internal-event-token=test-internal-token",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!",
        "app.security.policy.enforce-https=false"
    ]
)
class SecurityConfigTests(
    @Autowired private val applicationContext: ApplicationContext
) {
    private val webTestClient: WebTestClient by lazy {
        WebTestClient.bindToApplicationContext(applicationContext)
            .apply(springSecurity())
            .configureClient()
            .build()
    }

    @Test
    fun `health endpoint is public`() {
        webTestClient.get()
            .uri("/actuator/health")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
            }
    }

    @Test
    fun `auth login endpoint is public`() {
        webTestClient.post()
            .uri("/v1/auth/login")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"username":"demo","password":"demo"}""")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
            }
    }

    @Test
    fun `me endpoint requires authentication`() {
        webTestClient.get()
            .uri("/v1/me")
            .exchange()
            .expectStatus()
            .isUnauthorized
    }

    @Test
    fun `admin endpoint is forbidden for non admin role`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .get()
            .uri("/v1/admin/ping")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `posts list is public`() {
        webTestClient.get()
            .uri("/v1/posts")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
            }
    }

    @Test
    fun `post create requires organizer or admin`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_USER")))
            .post()
            .uri("/v1/posts")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"title":"t","content":"c"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `post delete requires admin`() {
        webTestClient.mutateWith(mockJwt().authorities(SimpleGrantedAuthority("ROLE_ORGANIZER")))
            .delete()
            .uri("/v1/posts/123")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `internal invalidation endpoint is forbidden without internal token`() {
        webTestClient.post()
            .uri("/internal/v1/cache/invalidation")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"eventType":"LOGOUT","subject":"user-1"}""")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `internal invalidation endpoint accepts valid internal token`() {
        webTestClient.post()
            .uri("/internal/v1/cache/invalidation")
            .header("X-Internal-Token", "test-internal-token")
            .contentType(MediaType.APPLICATION_JSON)
            .bodyValue("""{"eventType":"LOGOUT","subject":"user-1"}""")
            .exchange()
            .expectStatus()
            .isAccepted
    }
}
