package com.aandi.gateway.observability

import com.aandi.gateway.logging.ApiLogContext
import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import org.junit.jupiter.api.AfterAll
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.context.ApplicationContext
import org.springframework.http.MediaType
import org.springframework.security.test.web.reactive.server.SecurityMockServerConfigurers.springSecurity
import org.springframework.test.context.DynamicPropertyRegistry
import org.springframework.test.context.DynamicPropertySource
import org.springframework.test.web.reactive.server.WebTestClient
import java.net.InetSocketAddress
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

@SpringBootTest(
    properties = [
        "app.security.policy.enforce-https=false",
        "security.jwt.secret=test-secret-key-with-32-bytes-minimum!",
        "POST_SERVICE_URI=http://127.0.0.1:1",
        "ONLINE_JUDGE_SERVICE_URI=http://127.0.0.1:1"
    ]
)
class TraceIdRequestIdTest(
    @Autowired private val applicationContext: ApplicationContext
) {
    private val webTestClient: WebTestClient by lazy {
        WebTestClient.bindToApplicationContext(applicationContext)
            .apply(springSecurity())
            .configureClient()
            .build()
    }

    @BeforeEach
    fun resetDownstream() {
        downstream.reset()
    }

    @Test
    fun `gateway generates trace and request ids and forwards them to downstream`() {
        val result = webTestClient.get()
            .uri("/v2/ping")
            .exchange()
            .expectStatus().isOk
            .expectHeader().exists(ApiLogContext.TRACE_ID_HEADER)
            .expectHeader().exists(ApiLogContext.REQUEST_ID_HEADER)
            .expectHeader().contentTypeCompatibleWith(MediaType.APPLICATION_JSON)
            .expectBody(String::class.java)
            .returnResult()

        val responseTraceId = result.responseHeaders.getFirst(ApiLogContext.TRACE_ID_HEADER)
        val responseRequestId = result.responseHeaders.getFirst(ApiLogContext.REQUEST_ID_HEADER)
        val request = downstream.takeRequest()

        assertTrue(!responseTraceId.isNullOrBlank())
        assertTrue(!responseRequestId.isNullOrBlank())
        assertEquals(responseTraceId, request.header(ApiLogContext.TRACE_ID_HEADER))
        assertEquals(responseRequestId, request.header(ApiLogContext.REQUEST_ID_HEADER))
        assertEquals("/v2/ping", request.path)
    }

    @Test
    fun `gateway reuses incoming trace and request ids in response and downstream headers`() {
        val result = webTestClient.get()
            .uri("/v2/ping")
            .header(ApiLogContext.TRACE_ID_HEADER, "trace-observability-test")
            .header(ApiLogContext.REQUEST_ID_HEADER, "request-observability-test")
            .exchange()
            .expectStatus().isOk
            .expectHeader().valueEquals(ApiLogContext.TRACE_ID_HEADER, "trace-observability-test")
            .expectHeader().valueEquals(ApiLogContext.REQUEST_ID_HEADER, "request-observability-test")
            .expectBody(String::class.java)
            .returnResult()

        assertNotNull(result.responseBody)
        val request = downstream.takeRequest()

        assertEquals("trace-observability-test", request.header(ApiLogContext.TRACE_ID_HEADER))
        assertEquals("request-observability-test", request.header(ApiLogContext.REQUEST_ID_HEADER))
    }

    companion object {
        private val downstream = RecordingHttpServer.start()

        @JvmStatic
        @DynamicPropertySource
        fun downstreamProperties(registry: DynamicPropertyRegistry) {
            registry.add("AUTH_SERVICE_URI") { downstream.baseUrl }
        }

        @JvmStatic
        @AfterAll
        fun stopDownstream() {
            downstream.stop()
        }
    }
}

private data class RecordedRequest(
    val method: String,
    val path: String,
    val headers: Map<String, List<String>>
) {
    fun header(name: String): String? {
        return headers.entries
            .firstOrNull { it.key.equals(name, ignoreCase = true) }
            ?.value
            ?.firstOrNull()
    }
}

private class RecordingHttpServer private constructor(
    private val server: HttpServer
) {
    private val requests = LinkedBlockingQueue<RecordedRequest>()
    val baseUrl: String = "http://127.0.0.1:${server.address.port}"

    fun reset() {
        requests.clear()
    }

    fun takeRequest(): RecordedRequest {
        return requests.poll(2, TimeUnit.SECONDS) ?: error("mock downstream did not receive a request")
    }

    fun stop() {
        server.stop(0)
    }

    private fun handle(exchange: HttpExchange) {
        requests.add(
            RecordedRequest(
                method = exchange.requestMethod,
                path = exchange.requestURI.path,
                headers = exchange.requestHeaders.mapValues { it.value.toList() }
            )
        )
        val body = """{"success":true,"data":{"source":"mock-downstream"},"error":null}""".toByteArray()
        exchange.responseHeaders.add("Content-Type", "application/json")
        exchange.sendResponseHeaders(200, body.size.toLong())
        exchange.responseBody.use { it.write(body) }
    }

    companion object {
        fun start(): RecordingHttpServer {
            val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
            val recordingServer = RecordingHttpServer(server)
            server.createContext("/") { exchange -> recordingServer.handle(exchange) }
            server.start()
            return recordingServer
        }
    }
}
