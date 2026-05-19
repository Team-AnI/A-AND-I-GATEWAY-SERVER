package com.aandi.gateway.security

import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.context.ApplicationContext
import org.springframework.http.HttpHeaders
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.springSecurity
import org.springframework.test.web.reactive.server.WebTestClient
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue

@SpringBootTest(
    properties = [
        "POST_SERVICE_URI=http://localhost:8084",
        "AUTH_SERVICE_URI=http://localhost:9000",
        "ONLINE_JUDGE_SERVICE_URI=http://localhost:8080",
        "app.security.internal-event-token=test-internal-token",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!",
        "app.security.policy.enforce-https=true",
        "app.security.policy.allowed-hosts=api.aandiclub.com",
        "app.security.policy.allow-private-ip-host=false"
    ]
)
class ActuatorHealthPolicyTests(
    @Autowired private val applicationContext: ApplicationContext
) {
    private val webTestClient: WebTestClient by lazy {
        WebTestClient.bindToApplicationContext(applicationContext)
            .apply(springSecurity())
            .configureClient()
            .build()
    }

    @Test
    fun `actuator health is available for docker internal host`() {
        webTestClient.get()
            .uri("/actuator/health")
            .header(HttpHeaders.HOST, "gateway:9090")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
            }
    }

    @Test
    fun `actuator health subpaths are available for docker internal host`() {
        webTestClient.get()
            .uri("/actuator/health/readiness")
            .header(HttpHeaders.HOST, "gateway:9090")
            .exchange()
            .expectStatus()
            .value {
                assertNotEquals(401, it)
                assertNotEquals(403, it)
            }
    }

    @Test
    fun `non health actuator endpoints do not bypass host policy`() {
        webTestClient.get()
            .uri("/actuator/info")
            .header(HttpHeaders.HOST, "gateway:9090")
            .exchange()
            .expectStatus()
            .isForbidden
    }

    @Test
    fun `non health actuator endpoints are not public with allowed host`() {
        webTestClient.get()
            .uri("/actuator/info")
            .header(HttpHeaders.HOST, "api.aandiclub.com")
            .header("X-Forwarded-Proto", "https")
            .exchange()
            .expectStatus()
            .value {
                assertTrue(it == 401 || it == 404)
            }
    }
}
