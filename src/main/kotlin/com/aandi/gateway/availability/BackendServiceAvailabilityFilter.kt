package com.aandi.gateway.availability

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponseWriter
import org.slf4j.LoggerFactory
import org.springframework.core.Ordered
import org.springframework.http.HttpMethod
import org.springframework.stereotype.Component
import org.springframework.web.server.ServerWebExchange
import org.springframework.web.server.WebFilter
import org.springframework.web.server.WebFilterChain
import reactor.core.publisher.Mono

@Component
class BackendServiceAvailabilityFilter(
    private val availabilityService: BackendServiceAvailabilityService,
    private val responseWriter: GatewayResponseWriter
) : WebFilter, Ordered {

    private val log = LoggerFactory.getLogger(javaClass)

    override fun getOrder(): Int = Ordered.HIGHEST_PRECEDENCE + 25

    override fun filter(exchange: ServerWebExchange, chain: WebFilterChain): Mono<Void> {
        if (exchange.request.method == HttpMethod.OPTIONS) {
            return chain.filter(exchange)
        }

        val path = exchange.request.path.pathWithinApplication()
        val service = BackendService.entries.firstOrNull { it.matches(path) }
            ?: return chain.filter(exchange)

        return availabilityService.isEnabled(service).flatMap { enabled ->
            if (enabled) {
                chain.filter(exchange)
            } else {
                log.info(
                    "Rejecting request due to backend availability toggle: service={}, method={}, path={}",
                    service.name,
                    exchange.request.method,
                    path.value()
                )
                responseWriter.writeError(exchange, GatewayErrorCode.SERVICE_TEMPORARILY_DISABLED)
            }
        }
    }
}
