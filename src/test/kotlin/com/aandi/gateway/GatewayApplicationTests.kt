package com.aandi.gateway

import org.junit.jupiter.api.Test
import org.springframework.boot.test.context.SpringBootTest

@SpringBootTest(
	properties = [
		"POST_SERVICE_URI=http://localhost:8084",
		"app.security.internal-event-token=test-internal-token"
	]
)
class GatewayApplicationTests {

	@Test
	fun contextLoads() {
	}

}
