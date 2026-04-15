package com.aandi.gateway.logging

import com.aandi.gateway.common.response.GatewayErrorCode
import com.aandi.gateway.common.response.GatewayResponseWriter
import org.springframework.boot.webflux.error.ErrorWebExceptionHandler
import org.springframework.core.Ordered
import org.springframework.http.HttpStatus
import org.springframework.security.core.Authentication
import org.springframework.stereotype.Component
import org.springframework.web.server.ResponseStatusException
import org.springframework.web.server.ServerWebExchange
import reactor.core.publisher.Mono
import java.security.Principal
import java.util.Optional

@Component
class GlobalExceptionHandler(
    private val responseWriter: GatewayResponseWriter,
    private val apiLogFactory: ApiLogFactory,
    private val apiStructuredLogger: ApiStructuredLogger
) : ErrorWebExceptionHandler, Ordered {

    override fun getOrder(): Int = -2

    override fun handle(exchange: ServerWebExchange, ex: Throwable): Mono<Void> {
        if (exchange.response.isCommitted) {
            return Mono.error(ex)
        }

        val errorCode = mapErrorCode(ex)
        val context = ApiLogContext.get(exchange)
        context.markFailure("${ex.javaClass.simpleName}: ${ex.message ?: errorCode.message}")

        return responseWriter.writeError(exchange, errorCode)
            .then(resolveAuthentication(exchange))
            .doOnNext { authentication ->
                apiStructuredLogger.log(
                    apiLogFactory.createExceptionLog(
                        exchange = exchange,
                        context = context,
                        authentication = authentication.orElse(null),
                        errorCode = errorCode,
                        throwable = ex
                    )
                )
            }
            .then()
    }

    private fun mapErrorCode(ex: Throwable): GatewayErrorCode {
        return when (ex) {
            is ResponseStatusException -> when (ex.statusCode.value()) {
                HttpStatus.NOT_FOUND.value() -> GatewayErrorCode.ENDPOINT_NOT_ALLOWLISTED
                else -> GatewayErrorCode.INTERNAL_SERVER_ERROR
            }
            else -> GatewayErrorCode.INTERNAL_SERVER_ERROR
        }
    }

    private fun resolveAuthentication(exchange: ServerWebExchange): Mono<Optional<Authentication>> {
        return exchange.getPrincipal<Principal>()
            .map { Optional.ofNullable(it as? Authentication) }
            .switchIfEmpty(Mono.just(Optional.empty()))
            .onErrorReturn(Optional.empty())
    }
}
